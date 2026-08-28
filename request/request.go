package request

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	authpkg "github.com/zebodotdev/httpapi/auth"
	"github.com/zebodotdev/httpapi/cost"
	e "github.com/zebodotdev/httpapi/erreur"
	"github.com/zebodotdev/httpapi/response"
)

const (
	authTypeBearer  = authpkg.TypeBearer
	authTypeService = authpkg.TypeService
	unAuthzReqAppID = "UNAUTHORIZED_REQUEST"

	// intentionally case-insensitive. we're accessing
	// the value through `r.Header.Get` method, which
	// doesn't respect the case anyways.
	contentTypeHeaderKey = "content-type"
	originHeaderKey      = "origin"
	authHeaderKey        = "authorization"
	fwdAuthHeaderKey     = "x-forwarded-authorization"
	idempotencyHeaderKey = "idempotency-key"
	xReqIDHeaderKey      = "x-request-id"
	xReqTimingHeaderKey  = "x-request-timing"
	traceParentHeaderKey = "traceparent"

	corsOriginHeaderKey  = "access-control-allow-origin"
	corsMethodsHeaderKey = "access-control-allow-methods"
	corsHeadersHeaderKey = "access-control-allow-headers"

	// ReqTable is the default request audit table name used by services that
	// persist httpapi request audits.
	ReqTable = "api_requests"

	// ReqPartKeyK is the request audit partition-key attribute name.
	ReqPartKeyK = "application_id"

	// ReqSortKeyK is the request audit sort-key attribute name.
	ReqSortKeyK = "id"

	// IdType is the request ID prefix used by NewID.
	IdType = "req"

	// RequestRetention is how long request audit records remain valid.
	RequestRetention = 30 * 24 * time.Hour
)

// UnauthorizedAppID is the placeholder application id assigned before request
// authentication succeeds.
const UnauthorizedAppID = unAuthzReqAppID

var logr = log.New(os.Stdout, "[httpapi/request]: ", log.Flags()|log.Llongfile)

type jsonb map[string]string

// Req is the parsed request object passed to endpoint handlers.
//
// Req deliberately represents a safe parse of the incoming HTTP request. It
// does not know which endpoint will handle it or what that endpoint requires;
// endpoint authorization, internal-only policy, idempotency, priority, and
// timeout expectations live on endpoint.Endpoint and endpoint.EndpointSpec.
type Req struct {
	// AppID is the authenticated application id, or UnauthorizedAppID before a
	// session has been attached.
	AppID string `json:"application_id"`

	// SessID is the authenticated session id.
	SessID string `json:"session_id,omitempty"`

	// IdemKey is the idempotency key used by idempotent endpoints. It is
	// redacted during audit serialization.
	IdemKey string `json:"idempotency_key,omitzero"`

	// RecdAt is when httpapi received and began parsing the request.
	RecdAt time.Time `json:"received_at"`

	// ExpiresAt is when the persisted request audit record should expire.
	ExpiresAt time.Time `json:"expires_at"`

	// ExpiresAtUnix is the DynamoDB TTL attribute in epoch seconds.
	ExpiresAtUnix int64 `json:"expires_at_unix"`

	// Req is the original Go HTTP request with its body restored for downstream
	// consumers after httpapi has buffered it.
	Req *http.Request `json:"request,omitempty"`

	// Body is the buffered request body.
	Body []byte `json:"body"`

	// Res is the response selected by the endpoint handler or runtime.
	Res *response.Res `json:"response,omitempty"`

	// Err is an optional structured error attached by callers for audit output.
	Err *e.Error `json:"error,omitempty"`

	// Dur is the completed request duration.
	Dur time.Duration `json:"duration,omitzero"`

	// ID is the httpapi request identifier.
	ID string `json:"id"`

	// Sess is the authenticated session attached by middleware, context, or the
	// configured Authenticator.
	Sess *Session `json:"session"`

	// Caller is the application-defined request source attached by trusted
	// middleware or tests.
	Caller Caller `json:"caller"`

	// AuthFailure records credential parsing or authentication failure details
	// for audit output.
	AuthFailure *AuthFailure `json:"auth_failure,omitempty"`

	// AuthorizationFailure records endpoint access-policy failure details for
	// audit output.
	AuthorizationFailure *AuthFailure `json:"authorization_failure,omitempty"`

	costRecorder *cost.Recorder
}

// Res is the response type produced by request handlers.
type Res = response.Res

func (r *Req) partKey() string { return r.AppID }

// SetResponse stores the response that should be written for the request.
func (r *Req) SetResponse(res *response.Res) {
	if r == nil {
		return
	}
	r.Res = res
}

// Response returns the response currently attached to the request.
func (r *Req) Response() *response.Res {
	if r == nil {
		return nil
	}
	return r.Res
}

// MarshalJSON serializes Req into a redacted audit-safe JSON shape.
//
// Sensitive headers, request bodies, response bodies, authorization material,
// and idempotency keys are redacted or summarized before serialization.
func (r Req) MarshalJSON() ([]byte, error) {
	r.ensureRequestExpiration()
	safeReq, safeh, reqURL := r.auditRequest()
	bdy := auditBody(r.Body)
	res := r.auditResponse()

	return json.Marshal(struct {
		URL                  string         `json:"url"`
		AppID                string         `json:"application_id"`
		SessID               string         `json:"session_id,omitempty"`
		IdemKey              string         `json:"idempotency_key,omitzero"`
		RecdAt               time.Time      `json:"received_at"`
		ExpiresAt            time.Time      `json:"expires_at"`
		ExpiresAtUnix        int64          `json:"expires_at_unix"`
		Header               http.Header    `json:"header,omitempty"`
		Req                  *RequestAudit  `json:"request,omitempty"`
		Body                 any            `json:"body,omitempty"`
		Res                  *ResponseAudit `json:"response,omitempty"`
		Err                  *e.Error       `json:"error,omitempty"`
		Dur                  time.Duration  `json:"duration"`
		ID                   string         `json:"id"`
		Caller               Caller         `json:"caller"`
		Auth                 AuthAudit      `json:"auth"`
		AuthFailure          *AuthFailure   `json:"auth_failure,omitempty"`
		AuthorizationFailure *AuthFailure   `json:"authorization_failure,omitempty"`
		Session              *SessionAudit  `json:"session,omitempty"`
	}{
		URL:                  reqURL,
		AppID:                r.AppID,
		SessID:               r.SessID,
		IdemKey:              auditSecret(r.IdemKey),
		RecdAt:               r.RecdAt,
		ExpiresAt:            r.ExpiresAt,
		ExpiresAtUnix:        r.ExpiresAtUnix,
		Body:                 bdy,
		Header:               safeh,
		Req:                  safeReq,
		Res:                  res,
		Err:                  r.Err,
		Dur:                  r.Dur,
		ID:                   r.ID,
		Caller:               r.Caller,
		Auth:                 r.authAudit(),
		AuthFailure:          r.AuthFailure,
		AuthorizationFailure: r.AuthorizationFailure,
		Session:              sessionAudit(r.Sess),
	})
}

func (r *Req) ensureRequestExpiration() {
	if r == nil {
		return
	}

	expiresAt := r.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = requestExpiration(r.RecdAt)
	}
	r.ExpiresAt = expiresAt.UTC()
	if r.ExpiresAtUnix <= 0 {
		r.ExpiresAtUnix = r.ExpiresAt.Unix()
	}
}

func requestExpiration(receivedAt time.Time) time.Time {
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}
	return receivedAt.UTC().Add(RequestRetention)
}

// Context returns the underlying request context or context.Background when the
// Req is nil or not backed by an http.Request.
func (r *Req) Context() context.Context {
	if r == nil {
		return context.Background()
	}
	if r.Req == nil {
		return cost.ContextWithRecorder(context.Background(), r.costRecorder)
	}

	ctx := r.Req.Context()
	if r.costRecorder != nil && cost.RecorderFromContext(ctx) == nil {
		return cost.ContextWithRecorder(ctx, r.costRecorder)
	}
	return ctx
}

// RequestAudit is the redacted HTTP request shape embedded in Req audit JSON.
type RequestAudit struct {
	// Method is the inbound HTTP method.
	Method string `json:"method,omitempty"`

	// URL is the request URL with query string and fragment removed.
	URL string `json:"url,omitempty"`

	// Path is the request URL path.
	Path string `json:"path,omitempty"`

	// RequestURI is the escaped path without query parameters.
	RequestURI string `json:"request_uri,omitempty"`

	// Host is the inbound Host value.
	Host string `json:"host,omitempty"`

	// RemoteAddr is the remote address observed by the Go HTTP server.
	RemoteAddr string `json:"remote_addr,omitempty"`

	// UserAgent is the inbound User-Agent value.
	UserAgent string `json:"user_agent,omitempty"`

	// Referer is the redacted Referer URL.
	Referer string `json:"referer,omitempty"`

	// Header contains a small allowlist of safe headers plus redacted sensitive
	// headers.
	Header http.Header `json:"header,omitempty"`

	// QueryRedacted reports whether query parameters were present and removed.
	QueryRedacted bool `json:"query_redacted,omitempty"`
}

// ResponseAudit is the redacted HTTP response shape embedded in Req audit JSON.
type ResponseAudit struct {
	// ContentType is the response media type.
	ContentType string `json:"content_type,omitempty"`

	// Status is the HTTP response status code.
	Status int `json:"status,omitempty"`

	// SentAt is when the response object was created.
	SentAt time.Time `json:"sent_at,omitzero"`

	// Header contains response headers safe for audit output.
	Header http.Header `json:"header,omitempty"`

	// BodyPresent records whether a non-empty body or body reader was attached.
	BodyPresent bool `json:"body_present,omitempty"`

	// Streamed records whether the response body was streamed.
	Streamed bool `json:"streamed,omitempty"`
}

func (r Req) auditRequest() (*RequestAudit, http.Header, string) {
	if r.Req == nil {
		return nil, nil, ""
	}

	header := auditHeader(r.Req.Header)
	url := auditURLString(r.Req)
	path := ""
	queryRedacted := false
	if r.Req.URL != nil {
		path = r.Req.URL.Path
		queryRedacted = r.Req.URL.RawQuery != ""
	}

	audit := &RequestAudit{
		Method:        r.Req.Method,
		URL:           url,
		Path:          path,
		RequestURI:    auditRequestURI(r.Req),
		Host:          r.Req.Host,
		RemoteAddr:    r.Req.RemoteAddr,
		UserAgent:     r.Req.UserAgent(),
		Referer:       auditRawURLString(r.Req.Referer()),
		Header:        header,
		QueryRedacted: queryRedacted,
	}

	return audit, header, url
}

func (r Req) auditResponse() *ResponseAudit {
	if r.Res == nil {
		return nil
	}

	return &ResponseAudit{
		ContentType: r.Res.ContentType,
		Status:      r.Res.Status,
		SentAt:      r.Res.SentAt,
		Header:      auditResponseHeader(r.Res.Header),
		BodyPresent: r.Res.Body != nil || r.Res.BodyReader != nil,
		Streamed:    r.Res.BodyReader != nil,
	}
}

func auditHeader(header http.Header) http.Header {
	out := make(http.Header)
	for _, key := range []string{
		contentTypeHeaderKey,
		"user-agent",
		"accept",
		originHeaderKey,
		xReqIDHeaderKey,
		xReqTimingHeaderKey,
		traceParentHeaderKey,
	} {
		copyHeader(out, header, key)
	}
	redactHeader(out, header, authHeaderKey)
	redactHeader(out, header, fwdAuthHeaderKey)
	redactHeader(out, header, idempotencyHeaderKey)
	redactHeader(out, header, "cookie")
	redactHeader(out, header, "proxy-authorization")
	redactHeader(out, header, "x-api-key")
	return out
}

func auditResponseHeader(header http.Header) http.Header {
	out := make(http.Header)
	for _, key := range []string{
		contentTypeHeaderKey,
		"cache-control",
		xReqIDHeaderKey,
		xReqTimingHeaderKey,
	} {
		copyHeader(out, header, key)
	}
	for _, key := range []string{
		"location",
		"set-cookie",
	} {
		redactHeader(out, header, key)
	}
	return out
}

func copyHeader(dst, src http.Header, key string) {
	if strings.TrimSpace(src.Get(key)) == "" {
		return
	}
	dst.Set(key, src.Get(key))
}

func redactHeader(dst, src http.Header, key string) {
	if strings.TrimSpace(src.Get(key)) == "" {
		return
	}

	dst.Set(key, "REDACTED")
}

func auditBody(body []byte) any {
	if len(body) == 0 {
		return nil
	}
	return "REDACTED"
}

func auditSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "REDACTED"
}

func auditURLString(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	u := auditURL(*req.URL)
	return u.String()
}

func auditRawURLString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "REDACTED"
	}
	safeURL := auditURL(*u)
	return safeURL.String()
}

func auditURL(u url.URL) url.URL {
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	return u
}

func auditRequestURI(req *http.Request) string {
	if req == nil || req.URL == nil {
		return ""
	}
	return req.URL.EscapedPath()
}

// Origin returns the request Origin header.
func (r *Req) Origin() string { return r.Req.Header.Get("origin") }

// Path returns the parsed URL path.
func (r *Req) Path() string { return r.Req.URL.Path }

// URL returns the full request URL string.
func (r *Req) URL() string { return r.Req.URL.String() }

// ContentType returns the request Content-Type header.
func (r *Req) ContentType() string { return r.Req.Header.Get("content-type") }

// Authorization returns the credential-bearing Authorization value.
//
// X-Forwarded-Authorization is preferred when present so a trusted proxy can
// forward credentials while using Authorization for its own hop.
func (r *Req) Authorization() string {
	auth := strings.TrimSpace(r.Req.Header.Get(fwdAuthHeaderKey))
	if auth == "" {
		auth = r.Req.Header.Get(authHeaderKey)
	}
	return auth
}

// DownstreamAuthorization returns the Authorization value that should be sent
// to a downstream service.
//
// Service sessions may carry a bearer token representing the delegated subject;
// in that case this method converts the session token back into the configured
// bearer scheme. Regular requests return their original Authorization value.
func (r *Req) DownstreamAuthorization() string {
	if r == nil {
		return ""
	}

	if r.Sess != nil && r.Sess.ServiceSession() {
		token := strings.TrimSpace(r.Sess.Token)
		if token == "" {
			return ""
		}
		return currentAuthorizationSchemes().Bearer + " " + token
	}

	return r.Authorization()
}

// RemoteAddr returns the remote network address observed by the Go HTTP
// server.
func (r *Req) RemoteAddr() string { return r.Req.RemoteAddr }

// Method returns the inbound HTTP method.
func (r *Req) Method() string { return r.Req.Method }

// Referer returns the request Referer header.
func (r *Req) Referer() string { return r.Req.Referer() }

// RequestBody returns the buffered request body bytes.
func (r *Req) RequestBody() []byte {
	if r == nil {
		return nil
	}
	return r.Body
}

// AddCostUsage records one provider-neutral usage unit for the request.
//
// AddCostUsage is intended for request handlers and helper packages that
// perform metered work while serving the request. The request owns the
// underlying recorder so endpoint authors do not have to thread a separate
// collector through domain calls that already receive *request.Req,
// *endpoint.Req, or context.Context. The endpoint completion event includes the
// final usage snapshot for service-owned cost sinks.
func (r *Req) AddCostUsage(usage cost.UsageUnit) error {
	if r == nil {
		return fmt.Errorf("httpapi/request: cannot add cost usage to nil request")
	}
	return r.ensureCostRecorder().Record(usage)
}

// AddCostUnit records one provider-neutral usage unit from its required
// fields.
//
// AddCostUnit is a compact convenience around AddCostUsage for common cases
// where the caller does not need labels or a custom observation timestamp.
func (r *Req) AddCostUnit(
	provider cost.Provider,
	service cost.Service,
	sku cost.SKU,
	unit cost.Unit,
	quantity cost.Quantity,
) error {
	if r == nil {
		return fmt.Errorf("httpapi/request: cannot add cost usage to nil request")
	}
	return r.ensureCostRecorder().AddUnit(provider, service, sku, unit, quantity)
}

// CostUsage returns a copy of the usage units recorded for the request.
//
// The returned slice and label maps are safe for callers to modify. Mutating
// them does not change the request's captured usage.
func (r *Req) CostUsage() []cost.UsageUnit {
	if r == nil || r.costRecorder == nil {
		return nil
	}
	return r.costRecorder.Usage()
}

// CostRecorder returns the request operation recorder, creating and attaching
// one when needed.
func (r *Req) CostRecorder() *cost.Recorder {
	if r == nil {
		return nil
	}
	return r.ensureCostRecorder()
}

func (r *Req) ensureCostRecorder() *cost.Recorder {
	if r.costRecorder != nil {
		return r.costRecorder
	}

	operation := r.defaultCostOperation()
	recorder := cost.NewRecorder(operation)
	if r.Req != nil {
		if parent := cost.RecorderFromContext(r.Req.Context()); parent != nil {
			recorder = cost.NewChildRecorder(parent, operation)
		}
		r.Req = r.Req.WithContext(cost.ContextWithRecorder(r.Req.Context(), recorder))
	}
	r.costRecorder = recorder
	return r.costRecorder
}

func (r *Req) defaultCostOperation() cost.Operation {
	operationID := strings.TrimSpace(r.ID)
	if operationID == "" {
		operationID = cost.NewOperationID()
	}

	startedAt := r.RecdAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}

	return cost.Operation{
		ID:                 operationID,
		RootID:             operationID,
		TraceID:            traceIDFromRequest(r.Req),
		CausationRequestID: causationRequestID(r.Req, operationID),
		Kind:               "http_request",
		StartedAt:          startedAt,
	}
}

func causationRequestID(req *http.Request, fallback string) string {
	if req == nil {
		return fallback
	}
	if requestID := strings.TrimSpace(req.Header.Get(xReqIDHeaderKey)); requestID != "" {
		return requestID
	}
	return fallback
}

func traceIDFromRequest(req *http.Request) string {
	if req == nil {
		return ""
	}
	return traceIDFromTraceparent(req.Header.Get(traceParentHeaderKey))
}

func traceIDFromTraceparent(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) < 4 {
		return ""
	}
	traceID := strings.ToLower(parts[1])
	if !validTraceID(traceID) {
		return ""
	}
	return traceID
}

func validTraceID(value string) bool {
	if len(value) != 32 {
		return false
	}

	allZero := true
	for _, r := range value {
		if r != '0' {
			allZero = false
		}
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return !allZero
}

// Errored reports whether a structured error has been attached to the request.
func (r *Req) Errored() bool { return r.Err != nil }

// Duration returns the elapsed milliseconds between request receipt and
// response creation. It returns nil until a response exists.
func (r *Req) Duration() *int64 {
	if r.Res == nil {
		return nil
	}

	diff := r.Res.SentAt.UnixMilli() - r.RecdAt.UnixMilli()
	return &diff
}

// UserAgent returns the request User-Agent, defaulting to unknown_user_agent
// when the header is empty.
func (r *Req) UserAgent() string {
	ua := strings.TrimSpace(r.Req.UserAgent())
	if ua == "" {
		ua = "unknown_user_agent"
	}

	return ua
}

// ResponseHeaders returns response headers as a JSON-friendly map. Multiple
// header values are joined with |.
func (r *Req) ResponseHeaders() *jsonb {
	if r.Res == nil {
		return nil
	}
	combined := make(jsonb)
	for k, v := range r.Res.Header {
		combined[k] = strings.Join(
			v,
			"|",
		)
	}

	return &combined
}

// ResponseBody returns the encoded non-streaming response body.
//
// Streaming responses cannot be buffered and return an error.
func (r *Req) ResponseBody() ([]byte, error) {
	if r.Res.BodyReader != nil {
		return nil, fmt.Errorf("httpapi: streamed response body cannot be buffered")
	}
	body, err := response.EncodeResponseBodyForCaller(r.Res, r.RequestCaller())
	if err != nil {
		logr.Printf(
			"failed to encode response as json:"+
				" request_id=%s error=%v",
			r.ID,
			err,
		)
		return nil, err
	}

	return body, nil
}

// ResponseContentType returns the response content type or nil when no response
// has been attached.
func (r *Req) ResponseContentType() *string {
	if r.Res == nil {
		return nil
	}

	return &r.Res.ContentType
}

// ResponseStatus returns the response status code or nil when no response has
// been attached.
func (r *Req) ResponseStatus() *int {
	if r.Res == nil {
		return nil
	}

	return &r.Res.Status
}

// Headers returns all request headers as a JSON-friendly map. Multiple header
// values are joined with " | ".
func (r *Req) Headers() jsonb {
	combined := make(jsonb)
	for k, v := range r.Req.Header {
		combined[k] = strings.Join(
			v,
			" | ",
		)
	}
	return combined
}

func genReqID() string {
	buf := make([]byte, 20)
	if _, err := rand.Read(buf); err == nil {
		return "req_" + hex.EncodeToString(buf)
	}

	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}

// NewID returns a new request identifier.
func NewID() string {
	return genReqID()
}

// NewReq parses an http.Request into an httpapi Req.
//
// NewReq buffers and restores the request body, assigns a request id, attaches
// any pre-authenticated context session, and attempts authentication when the
// Authorization scheme matches the configured bearer or service schemes. It
// returns nil when the request body cannot be read.
func NewReq(req *http.Request) *Req {
	parsed, err := NewReqWithError(req)
	if err != nil {
		return nil
	}
	return parsed
}

// NewReqWithError parses an http.Request into an httpapi Req and returns body
// read errors to the caller.
//
// NewReqWithError is useful for endpoint runtimes that need to distinguish a
// malformed body from transport-level read failures such as request-size limits.
func NewReqWithError(req *http.Request) (*Req, error) {
	if req.Body == nil {
		req.Body = http.NoBody
	}

	body, err := io.ReadAll(req.Body)
	if err != nil && err != io.EOF {
		logr.Printf(
			"error while reading request body:"+
				" remote_addr=%s user_agent=%s method=%s"+
				" request_url=%s content_type=%s error=%v",
			req.RemoteAddr, req.UserAgent(), req.Method,
			req.URL.String(), req.Header.Get("content-type"),
			err,
		)

		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))

	receivedAt := time.Now().UTC()
	expiresAt := requestExpiration(receivedAt)
	r := Req{
		AppID:         unAuthzReqAppID,
		RecdAt:        receivedAt,
		ExpiresAt:     expiresAt,
		ExpiresAtUnix: expiresAt.Unix(),
		Req:           req,
		Body:          body,
		ID:            genReqID(),
	}
	r.ensureCostRecorder()
	if session := SessionFromContext(req.Context()); session != nil {
		r.AttachSession(session)
	}
	if caller := CallerFromContext(req.Context()); caller.Defined() {
		r.AttachCaller(caller)
	}

	if r.Sess != nil {
		r.annotateContext()
		return &r, nil
	}

	auth := r.Authorization()
	authScheme, _, _ := strings.Cut(auth, " ")
	schemes := currentAuthorizationSchemes()
	switch strings.ToLower(authScheme) {
	case strings.ToLower(schemes.Bearer):
		if err := r.authenticate(authTypeBearer); err != nil {
			r.recordAuthFailure(authTypeBearer, err)
			logr.Printf(
				"error while attempting to authenticate request:"+
					" remote_addr=%s user_agent=%s method=%s"+
					" request_url=%s content_type=%s error=%v",
				req.RemoteAddr, req.UserAgent(), req.Method,
				req.URL.String(), req.Header.Get("content-type"),
				err,
			)
		}
	case serviceAuthorizationSchemeCase(schemes, authScheme):
		if err := r.authenticate(authTypeService); err != nil {
			r.recordAuthFailure(authTypeService, err)
			logr.Printf(
				"error while attempting to authenticate service request:"+
					" remote_addr=%s user_agent=%s method=%s"+
					" request_url=%s content_type=%s error=%v",
				req.RemoteAddr, req.UserAgent(), req.Method,
				req.URL.String(), req.Header.Get("content-type"),
				err,
			)
		}
	default:
		if strings.TrimSpace(auth) != "" {
			r.recordAuthFailure(authTypeUnknown, ErrUnsupportedAuthorizationScheme)
		}
	}

	r.annotateContext()
	return &r, nil
}

func serviceAuthorizationSchemeCase(schemes AuthorizationSchemes, candidate string) string {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	for _, scheme := range schemes.ServiceAuthorizationSchemes() {
		if candidate == strings.ToLower(scheme) {
			return candidate
		}
	}
	return "\x00"
}
