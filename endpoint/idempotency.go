package endpoint

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	e "github.com/zebodotdev/httpapi/erreur"
	requestpkg "github.com/zebodotdev/httpapi/request"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

const (
	idempotencyHeaderKey      = "idempotency-key"
	idempotencyReplayHeader   = "Idempotent-Replayed"
	idempotencyStatusReserved = "reserved"
	idempotencyStatusComplete = "complete"
	idempotencyRetention      = 24 * time.Hour
)

// RequestMeta is the standard request metadata envelope recognized by the
// endpoint runtime. It remains an endpoint alias for compatibility; new code
// may use request.Meta directly.
type RequestMeta = requestpkg.Meta

type idempotencyRequest struct {
	Scope       string
	Key         string
	Fingerprint string
	ExpiresAt   time.Time
}

// IdempotencyScopeResolver computes the storage scope for a request's
// idempotency key.
//
// Return an ErrInvalidParam when the request cannot safely produce a scope. The
// default scope uses the service namespace, endpoint method, endpoint pattern,
// and authenticated app/session context.
type IdempotencyScopeResolver func(*Req) (string, *e.ErrInvalidParam)

// IdempotencyRecord is the storage representation for one idempotency key.
type IdempotencyRecord struct {
	// Scope partitions idempotency keys so the same client key can be reused
	// for unrelated operations.
	Scope string `json:"scope"`

	// Key is the caller-provided idempotency key.
	Key string `json:"idempotency_key"`

	// Fingerprint is the normalized operation fingerprint used to reject key
	// reuse with a different request payload.
	Fingerprint string `json:"request_fingerprint"`

	// Status records whether the operation is reserved or complete.
	Status string `json:"status"`

	// RequestID is the httpapi request that reserved the key.
	RequestID string `json:"request_id,omitempty"`

	// ResponseStatus is the HTTP status code of the stored successful response.
	ResponseStatus int `json:"response_status,omitempty"`

	// ContentType is the response content type stored for replay.
	ContentType string `json:"content_type,omitempty"`

	// ResponseHeader stores replayable response headers.
	ResponseHeader http.Header `json:"response_header,omitempty"`

	// ResponseBody stores the encoded response body for replay.
	ResponseBody string `json:"response_body,omitempty"`

	// CreatedAt is when the reservation was created.
	CreatedAt time.Time `json:"created_at"`

	// CompletedAt is when the successful response was persisted.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// ExpiresAt is when the idempotency record may be discarded.
	ExpiresAt time.Time `json:"expires_at"`

	// ExpiresAtUnix is ExpiresAt as a Unix timestamp for stores that support TTL
	// indexes.
	ExpiresAtUnix int64 `json:"expires_at_unix"`
}

// IdempotencyStore is the persistence contract used by idempotent endpoints.
//
// Reserve must atomically create a reservation or return the existing record
// for the same scope/key. Complete stores a successful response for future
// replay. Release removes an incomplete reservation when the handler failed or
// produced a non-replayable response.
type IdempotencyStore interface {
	// Reserve atomically reserves a scope/key pair or returns the existing
	// record when another request already reserved it.
	Reserve(context.Context, *IdempotencyRecord) (*IdempotencyRecord, error)

	// Complete marks a reserved record complete and stores its replay response.
	Complete(context.Context, *IdempotencyRecord) error

	// Release removes an incomplete reservation.
	Release(context.Context, string, string) error
}

// ErrIdempotencyStoreNotConfigured is returned when an idempotent endpoint runs
// before ConfigureIdempotencyStore installs a concrete store.
var ErrIdempotencyStoreNotConfigured = errors.New("httpapi: idempotency_store_not_configured")

type unavailableIdempotencyStore struct{}

// Reserve fails because no concrete idempotency store is configured.
func (unavailableIdempotencyStore) Reserve(context.Context, *IdempotencyRecord) (*IdempotencyRecord, error) {
	return nil, ErrIdempotencyStoreNotConfigured
}

// Complete fails because no concrete idempotency store is configured.
func (unavailableIdempotencyStore) Complete(context.Context, *IdempotencyRecord) error {
	return ErrIdempotencyStoreNotConfigured
}

// Release fails because no concrete idempotency store is configured.
func (unavailableIdempotencyStore) Release(context.Context, string, string) error {
	return ErrIdempotencyStoreNotConfigured
}

var (
	idempotencyStoreMu sync.RWMutex
	idempotencyStore   IdempotencyStore = unavailableIdempotencyStore{}
	idempotencyScopeMu sync.RWMutex
	idempotencyScopeNS = "httpapi"
)

func currentIdempotencyStore() IdempotencyStore {
	idempotencyStoreMu.RLock()
	defer idempotencyStoreMu.RUnlock()
	return idempotencyStore
}

// ConfigureIdempotencyStore installs the package-level idempotency store.
//
// Idempotent endpoints fail closed until a store is configured. The returned
// function restores the previous store for tests and temporary overrides.
func ConfigureIdempotencyStore(store IdempotencyStore) func() {
	if store == nil {
		store = unavailableIdempotencyStore{}
	}

	idempotencyStoreMu.Lock()
	prev := idempotencyStore
	idempotencyStore = store
	idempotencyStoreMu.Unlock()

	return func() {
		idempotencyStoreMu.Lock()
		idempotencyStore = prev
		idempotencyStoreMu.Unlock()
	}
}

func setIdempotencyStoreForTest(store IdempotencyStore) func() {
	return ConfigureIdempotencyStore(store)
}

// ConfigureIdempotencyScopeNamespace sets the namespace used in default
// idempotency scopes.
//
// Services should set this to their service name so keys do not collide across
// binaries sharing the same IdempotencyStore.
func ConfigureIdempotencyScopeNamespace(namespace string) func() {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		namespace = "httpapi"
	}

	idempotencyScopeMu.Lock()
	prev := idempotencyScopeNS
	idempotencyScopeNS = namespace
	idempotencyScopeMu.Unlock()

	return func() {
		idempotencyScopeMu.Lock()
		idempotencyScopeNS = prev
		idempotencyScopeMu.Unlock()
	}
}

func currentIdempotencyScopeNamespace() string {
	idempotencyScopeMu.RLock()
	defer idempotencyScopeMu.RUnlock()
	return idempotencyScopeNS
}

func handleIdempotently(
	r *Req,
	meth HttpMethod,
	pattern string,
	handler Handler,
	resolvers ...IdempotencyScopeResolver,
) {
	resolver := firstIdempotencyScopeResolver(resolvers)
	if !r.Authorized() && resolver == nil {
		handler(r)
		return
	}

	ireq, perr := parseIdempotencyRequest(r, meth, pattern, resolver)
	if perr != nil {
		logIdempotencyEvent("validation_failed", r, "", "", "", perr.Code, 0)
		responsepkg.RenderParamErr(r, perr)
		return
	}

	store := currentIdempotencyStore()
	now := time.Now().UTC()
	record := &IdempotencyRecord{
		Scope:         ireq.Scope,
		Key:           ireq.Key,
		Fingerprint:   ireq.Fingerprint,
		Status:        idempotencyStatusReserved,
		RequestID:     r.ID,
		CreatedAt:     now,
		ExpiresAt:     ireq.ExpiresAt,
		ExpiresAtUnix: ireq.ExpiresAt.Unix(),
	}

	existing, err := store.Reserve(r.Context(), record)
	if err != nil {
		logr.Printf(
			"idempotency reservation failed:"+
				" request_id=%s scope_hash=%s key_hash=%s error=%v",
			r.ID, idempotencyKeyHash(ireq.Scope), idempotencyKeyHash(ireq.Key), err,
		)
		logIdempotencyEvent("store_unavailable", r, ireq.Scope, ireq.Key, ireq.Fingerprint, "", 0)
		renderIdempotencyStoreUnavailable(r)
		return
	}
	if existing != nil {
		replayOrRejectIdempotentRequest(r, existing, ireq)
		return
	}

	logIdempotencyEvent("reserved", r, ireq.Scope, ireq.Key, ireq.Fingerprint, "", 0)
	r.IdemKey = ireq.Key
	handler(r)
	if r.Res == nil || r.Res.Status < http.StatusOK || r.Res.Status > 299 || r.Res.BodyReader != nil {
		status := 0
		if r.Res != nil {
			status = r.Res.Status
		}
		if err := store.Release(r.Context(), ireq.Scope, ireq.Key); err != nil {
			logr.Printf(
				"idempotency reservation release failed:"+
					" request_id=%s scope_hash=%s key_hash=%s error=%v",
				r.ID, idempotencyKeyHash(ireq.Scope), idempotencyKeyHash(ireq.Key), err,
			)
		}
		logIdempotencyEvent("released_failure", r, ireq.Scope, ireq.Key, ireq.Fingerprint, "", status)
		return
	}

	body, err := r.ResponseBody()
	if err != nil {
		if err := store.Release(r.Context(), ireq.Scope, ireq.Key); err != nil {
			logr.Printf(
				"idempotency reservation release after response encode failed:"+
					" request_id=%s scope_hash=%s key_hash=%s error=%v",
				r.ID, idempotencyKeyHash(ireq.Scope), idempotencyKeyHash(ireq.Key), err,
			)
		}
		return
	}

	completedAt := time.Now().UTC()
	record.Status = idempotencyStatusComplete
	record.ResponseStatus = r.Res.Status
	record.ContentType = r.Res.ContentType
	record.ResponseHeader = cloneHeader(r.Res.Header)
	record.ResponseBody = strings.TrimRight(string(body), "\n")
	record.CompletedAt = &completedAt

	if err := store.Complete(r.Context(), record); err != nil {
		logr.Printf(
			"idempotency completion failed:"+
				" request_id=%s scope_hash=%s key_hash=%s error=%v",
			r.ID, idempotencyKeyHash(ireq.Scope), idempotencyKeyHash(ireq.Key), err,
		)
		logIdempotencyEvent("completion_failed", r, ireq.Scope, ireq.Key, ireq.Fingerprint, "", r.Res.Status)
		return
	}

	logIdempotencyEvent("completed", r, ireq.Scope, ireq.Key, ireq.Fingerprint, "", r.Res.Status)
}

func parseIdempotencyRequest(
	r *Req,
	meth HttpMethod,
	pattern string,
	resolvers ...IdempotencyScopeResolver,
) (*idempotencyRequest, *e.ErrInvalidParam) {
	key, perr := idempotencyKeyFromRequest(r)
	if perr != nil {
		return nil, perr
	}
	if key == "" {
		key, perr = idempotencyKeyFromSession(r)
		if perr != nil {
			return nil, perr
		}
	}
	if key == "" {
		generated, err := newIdempotencyKey()
		if err != nil {
			return nil, paramErrFromError("idempotency_key", e.IdempotencyKeyGenerationFailed())
		}
		key = generated
	}

	fingerprint, perr := canonicalOperationFingerprint(r.Body)
	if perr != nil {
		return nil, perr
	}

	scope, perr := idempotencyScopeForRequest(r, meth, pattern, firstIdempotencyScopeResolver(resolvers))
	if perr != nil {
		return nil, perr
	}

	r.IdemKey = key
	return &idempotencyRequest{
		Scope:       scope,
		Key:         key,
		Fingerprint: fingerprint,
		ExpiresAt:   time.Now().UTC().Add(idempotencyRetention),
	}, nil
}

func idempotencyKeyFromRequest(r *Req) (string, *e.ErrInvalidParam) {
	headerKey, perr := idempotencyKeyFromHeader(r)
	if perr != nil {
		return "", perr
	}
	if perr := rejectTopLevelIdempotencyKey(r.Body); perr != nil {
		return "", perr
	}

	metaKey, perr := normalizeIdempotencyKey(
		r.RequestMeta().IdempotencyKey,
		"request_meta.idempotency_key",
	)
	if perr != nil {
		return "", perr
	}

	if headerKey != "" && metaKey != "" && headerKey != metaKey {
		return "", idempotencyMismatchErr("Idempotency-Key", "request_meta.idempotency_key")
	}
	if headerKey != "" {
		return headerKey, nil
	}
	return metaKey, nil
}

func rejectTopLevelIdempotencyKey(body []byte) *e.ErrInvalidParam {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return e.InvalidBodyErr()
	}
	if _, ok := raw["idempotency_key"]; ok {
		return unsupportedIdempotencyKeyErr("idempotency_key")
	}
	return nil
}

func idempotencyKeyFromSession(r *Req) (string, *e.ErrInvalidParam) {
	if r == nil || r.Sess == nil {
		return "", nil
	}

	return normalizeIdempotencyKey(r.Sess.IdempotencyKey, "session.idempotency_key")
}

func newIdempotencyKey() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	return "idem_" + hex.EncodeToString(buf), nil
}

func idempotencyKeyFromHeader(r *Req) (string, *e.ErrInvalidParam) {
	if r == nil || r.Req == nil {
		return "", nil
	}

	raw := strings.TrimSpace(r.Req.Header.Get(idempotencyHeaderKey))
	if raw == "" {
		return "", nil
	}

	key := raw
	if strings.HasPrefix(raw, "\"") || strings.HasSuffix(raw, "\"") {
		if !strings.HasPrefix(raw, "\"") || !strings.HasSuffix(raw, "\"") {
			return "", invalidIdempotencyKeyErr("Idempotency-Key")
		}
		unquoted, err := strconv.Unquote(raw)
		if err != nil {
			return "", invalidIdempotencyKeyErr("Idempotency-Key")
		}
		key = unquoted
	}

	return normalizeIdempotencyKey(key, "Idempotency-Key")
}

func normalizeIdempotencyKey(raw, param string) (string, *e.ErrInvalidParam) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", nil
	}

	min, max := idempotencyKeyBounds()
	if len(key) < min || len(key) > max {
		return "", &e.ErrInvalidParam{
			Param: param,
			Mesg: fmt.Sprintf(
				"`%s` must be between %d and %d characters",
				param, min, max,
			),
			Code:    "idempotency_key_invalid",
			FixCode: e.FixCodeChangeParams,
		}
	}

	for _, rn := range key {
		if rn <= 0x20 || rn == 0x7f {
			return "", invalidIdempotencyKeyErr(param)
		}
	}

	return key, nil
}

func canonicalOperationFingerprint(body []byte) (string, *e.ErrInvalidParam) {
	if len(strings.TrimSpace(string(body))) == 0 {
		sum := sha256.Sum256([]byte("{}"))
		return hex.EncodeToString(sum[:]), nil
	}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var payload map[string]any
	if err := dec.Decode(&payload); err != nil {
		return "", e.InvalidBodyErr()
	}

	delete(payload, "request_meta")

	canon, err := json.Marshal(payload)
	if err != nil {
		return "", e.InvalidBodyErr()
	}

	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

func replayOrRejectIdempotentRequest(r *Req, existing *IdempotencyRecord, incoming *idempotencyRequest) {
	if existing.Status != idempotencyStatusComplete {
		logIdempotencyEvent("in_progress", r, incoming.Scope, incoming.Key, incoming.Fingerprint, "", 0)
		responsepkg.RenderErr(r, e.IdempotencyInProgress())
		return
	}

	if existing.Fingerprint != incoming.Fingerprint {
		logIdempotencyEvent("conflict", r, incoming.Scope, incoming.Key, incoming.Fingerprint, "", 0)
		responsepkg.RenderErr(r, e.IdempotencyConflict())
		return
	}

	header := cloneHeader(existing.ResponseHeader)
	if header == nil {
		header = make(http.Header)
	}
	header.Set(idempotencyReplayHeader, "true")

	r.IdemKey = incoming.Key
	logIdempotencyEvent("replayed", r, incoming.Scope, incoming.Key, incoming.Fingerprint, "", existing.ResponseStatus)
	r.Res = &Res{
		ContentType: existing.ContentType,
		Status:      existing.ResponseStatus,
		SentAt:      time.Now(),
		Header:      header,
		Body:        json.RawMessage(existing.ResponseBody),
	}
}

func renderIdempotencyStoreUnavailable(r *Req) {
	responsepkg.RenderErr(r, e.IdempotencyStorageUnavailable())
}

func idempotencyMismatchErr(first, second string) *e.ErrInvalidParam {
	return idempotencyParamErr(
		"idempotency_key",
		fmt.Sprintf("`%s` and `%s` must match when both are present", first, second),
		"idempotency_key_mismatch",
	)
}

func unsupportedIdempotencyKeyErr(param string) *e.ErrInvalidParam {
	return idempotencyParamErr(
		param,
		fmt.Sprintf("`%s` is not supported. use `request_meta.idempotency_key` or the `Idempotency-Key` header", param),
		"idempotency_key_unsupported",
	)
}

func invalidIdempotencyKeyErr(param string) *e.ErrInvalidParam {
	return idempotencyParamErr(
		param,
		fmt.Sprintf("`%s` is not a valid idempotency key", param),
		"idempotency_key_invalid",
	)
}

func idempotencyKeyBounds() (int, int) {
	return 1, 255
}

func idempotencyScope(r *Req, meth HttpMethod, pattern string) string {
	appID := r.AppID
	if r.Sess != nil && r.Sess.App.ID != "" {
		appID = r.Sess.App.ID
	}

	return idempotencyScopeWithAppID(r, meth, pattern, appID)
}

func idempotencyScopeForRequest(
	r *Req,
	meth HttpMethod,
	pattern string,
	resolver IdempotencyScopeResolver,
) (string, *e.ErrInvalidParam) {
	if resolver == nil {
		return idempotencyScope(r, meth, pattern), nil
	}

	appID, perr := resolver(r)
	if perr != nil {
		return "", perr
	}
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return "", paramErrFromError("application_id", e.ConfigurationMissing(
			"idempotency_scope_unavailable",
			"could not determine the application scope for this idempotent request",
			"the request could not be assigned to an application for idempotency. contact support with the request id if the problem persists.",
		))
	}

	if r != nil {
		r.AppID = appID
	}

	return idempotencyScopeWithAppID(r, meth, pattern, appID), nil
}

func idempotencyParamErr(param, mesg, code string) *e.ErrInvalidParam {
	return &e.ErrInvalidParam{
		Param:   param,
		Mesg:    mesg,
		Cause:   e.CauseInvalidParam,
		Type:    e.TypeIdempotency,
		Code:    code,
		FixCode: e.FixCodeChangeParams,
	}
}

func paramErrFromError(param string, err *e.Error) *e.ErrInvalidParam {
	return &e.ErrInvalidParam{
		Param:   param,
		Mesg:    err.Message,
		Status:  err.Status,
		Cause:   err.Cause,
		Type:    err.Type,
		Code:    err.Code,
		FixCode: err.FixCode,
	}
}

func idempotencyScopeWithAppID(r *Req, meth HttpMethod, pattern, appID string) string {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		appID = requestpkg.UnauthorizedAppID
	}

	path := ""
	if r != nil && r.Req != nil {
		path = r.Path()
	}
	if path == "" {
		path = pattern
	}

	return fmt.Sprintf(
		"%s#%s#%s#%s",
		appID,
		currentIdempotencyScopeNamespace(),
		strings.ToUpper(meth),
		path,
	)
}

func firstIdempotencyScopeResolver(resolvers []IdempotencyScopeResolver) IdempotencyScopeResolver {
	for _, resolver := range resolvers {
		if resolver != nil {
			return resolver
		}
	}
	return nil
}

func logIdempotencyEvent(event string, r *Req, scope, key, fingerprint, code string, status int) {
	requestID := ""
	path := ""
	method := ""
	if r != nil {
		requestID = r.ID
		if r.Req != nil {
			path = r.Path()
			method = r.Method()
		}
	}

	logr.Printf(
		"idempotency event:"+
			" metric=httpapi_idempotency_event service=%s event=%s"+
			" request_id=%s method=%s path=%s scope_hash=%s key_hash=%s"+
			" fingerprint=%s code=%s status=%d",
		currentIdempotencyScopeNamespace(), event, requestID, method, path,
		idempotencyKeyHash(scope), idempotencyKeyHash(key), fingerprint, code, status,
	)
}

func idempotencyKeyHash(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:16]
}

func (r *IdempotencyRecord) expired(now time.Time) bool {
	return r != nil && !r.ExpiresAt.IsZero() && r.ExpiresAt.Before(now)
}

func cloneHeader(header http.Header) http.Header {
	if header == nil {
		return nil
	}
	return header.Clone()
}

//go:fix inline
func stringPtr(v string) *string { return new(v) }
