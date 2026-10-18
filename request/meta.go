package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zebodotdev/httpapi/param"
	"github.com/zebodotdev/httpapi/response"
)

const (
	requestMetaName        = "request_meta"
	responseMetaName       = "response"
	projectionMetaName     = "projection"
	idempotencyKeyMetaName = "idempotency_key"
)

// Meta is the standard request_meta envelope accepted by httpapi endpoints.
// Endpoint-specific request parsers do not need to declare or parse it.
type Meta struct {
	// IdempotencyKey is the caller-provided idempotency key, when supplied in
	// the request body rather than the Idempotency-Key header.
	IdempotencyKey string `json:"idempotency_key,omitzero"`

	// Response contains caller preferences for computing and representing the
	// endpoint response.
	Response *ResponseMeta `json:"response,omitempty"`
}

// ResponseMeta contains standard response preferences carried by request_meta.
type ResponseMeta struct {
	// Projection selects public response properties by named resource.
	Projection ResponseProjection `json:"projection,omitzero"`
}

// ResponseProjection returns a defensive copy of the requested response
// property projection.
func (meta Meta) ResponseProjection() ResponseProjection {
	if meta.Response == nil {
		return ResponseProjection{}
	}
	return cloneResponseProjection(meta.Response.Projection)
}

type requestMetaParser struct {
	req       *Req
	envelope  map[string]json.RawMessage
	rawMeta   json.RawMessage
	meta      Meta
	resources map[string]PropertyProjection
	err       *param.Error
}

// PrepareMeta recognizes the standard request_meta envelope, validates the
// projection's generic wire syntax, attaches it to r, and removes request_meta
// from the body seen by endpoint-specific parsers.
//
// The endpoint runtime calls PrepareMeta before invoking application handlers.
// It then validates resource names and property paths against the endpoint's
// typed response contracts and caller visibility. Keeping those phases separate
// lets request parsing remain endpoint-agnostic while ensuring handlers receive
// only a semantically valid projection. Direct users of request.NewReq may call
// PrepareMeta explicitly, but must perform any endpoint-specific validation
// required by their own runtime.
func (r *Req) PrepareMeta() *param.Error {
	if r == nil || r.metaPrepared {
		return nil
	}
	body := bytes.TrimSpace(r.Body)
	if len(body) == 0 || !bytes.Contains(body, []byte(`"request_meta"`)) {
		r.metaPrepared = true
		return nil
	}
	parser := requestMetaParser{req: r}
	parser.decodeEnvelope()
	parser.extractMeta()
	parser.parseMeta()
	parser.stripMeta()
	parser.attach()
	return parser.err
}

// RequestMeta returns a defensive copy of the standard request metadata.
func (r *Req) RequestMeta() Meta {
	if r == nil {
		return Meta{}
	}
	return cloneMeta(r.meta)
}

// ResponseProjection returns the property projection requested through
// request_meta.response.projection.
func (r *Req) ResponseProjection() ResponseProjection {
	return r.RequestMeta().ResponseProjection()
}

// ResponseContext returns the authority and property selection used to render
// this request's response.
func (r *Req) ResponseContext() response.Context {
	if r == nil {
		return response.Context{}
	}
	return response.Context{
		Caller:     r.ResponseCaller(),
		Projection: r.ResponseProjection(),
	}
}

func (parser *requestMetaParser) decodeEnvelope() {
	if parser.err != nil || parser.req == nil || len(bytes.TrimSpace(parser.req.Body)) == 0 {
		return
	}

	if err := json.Unmarshal(parser.req.Body, &parser.envelope); err != nil {
		parser.err = param.NewError(
			"request_body",
			param.CodeInvalidBody,
			"request body must be a valid JSON object",
			err,
		)
		return
	}
	if parser.envelope == nil {
		parser.err = param.NewError(
			"request_body",
			param.CodeInvalidBody,
			"request body must be a valid JSON object",
			nil,
		)
	}
}

func (parser *requestMetaParser) extractMeta() {
	if parser.err != nil || parser.envelope == nil {
		return
	}
	parser.rawMeta = parser.envelope[requestMetaName]
}

func (parser *requestMetaParser) parseMeta() {
	if parser.err != nil || len(parser.rawMeta) == 0 || isJSONNull(parser.rawMeta) {
		return
	}

	object := parser.decodeObject(requestMetaName, parser.rawMeta)
	if parser.err != nil {
		return
	}
	parser.parseIdempotencyKey(object[idempotencyKeyMetaName])
	parser.parseResponse(object[responseMetaName])
}

func (parser *requestMetaParser) parseIdempotencyKey(raw json.RawMessage) {
	if parser.err != nil || len(raw) == 0 || isJSONNull(raw) {
		return
	}
	if err := json.Unmarshal(raw, &parser.meta.IdempotencyKey); err != nil {
		parser.typeMismatch(joinMetaPath(requestMetaName, idempotencyKeyMetaName), "a string", err)
	}
}

func (parser *requestMetaParser) parseResponse(raw json.RawMessage) {
	if parser.err != nil || len(raw) == 0 || isJSONNull(raw) {
		return
	}

	object := parser.decodeObject(joinMetaPath(requestMetaName, responseMetaName), raw)
	if parser.err != nil {
		return
	}
	parser.parseProjection(object[projectionMetaName])
	if parser.err == nil && len(parser.resources) > 0 {
		projection, err := defineResponseProjection(parser.resources)
		if err != nil {
			parser.invalid(projectionPath(), err)
			return
		}
		parser.meta.Response = &ResponseMeta{Projection: projection}
	}
}

func (parser *requestMetaParser) parseProjection(raw json.RawMessage) {
	if parser.err != nil || len(raw) == 0 || isJSONNull(raw) {
		return
	}

	resources := parser.decodeObject(projectionPath(), raw)
	if parser.err != nil {
		return
	}
	parser.resources = make(map[string]PropertyProjection, len(resources))
	for resource, selection := range resources {
		parser.parseResourceProjection(resource, selection)
		if parser.err != nil {
			return
		}
	}
}

func (parser *requestMetaParser) parseResourceProjection(
	resource string,
	raw json.RawMessage,
) {
	path := joinMetaPath(projectionPath(), resource)
	if err := validatePropertyName(strings.TrimSpace(resource)); err != nil {
		parser.invalid(path, err)
		return
	}
	selection := parser.decodeObject(path, raw)
	if parser.err != nil {
		return
	}

	for name := range selection {
		if name != string(ProjectionModePick) && name != string(ProjectionModeOmit) {
			parser.err = param.NewError(
				joinMetaPath(path, name),
				param.CodeUnexpected,
				fmt.Sprintf("`%s` is an unexpected parameter", joinMetaPath(path, name)),
				nil,
			)
			return
		}
	}

	pick, hasPick := selection[string(ProjectionModePick)]
	omit, hasOmit := selection[string(ProjectionModeOmit)]
	if hasPick && hasOmit {
		parser.err = param.NewError(
			path+".pick|"+path+".omit",
			param.CodeMutuallyExclusive,
			fmt.Sprintf("`%s.pick` and `%s.omit` are mutually exclusive; provide only one", path, path),
			nil,
		)
		return
	}
	if !hasPick && !hasOmit {
		parser.err = param.NewError(
			path,
			param.CodeRequiredChoice,
			fmt.Sprintf("`%s` must define either `pick` or `omit`", path),
			nil,
		)
		return
	}

	mode, rawPaths := ProjectionModePick, pick
	if hasOmit {
		mode, rawPaths = ProjectionModeOmit, omit
	}
	paths := parser.parsePropertyPaths(joinMetaPath(path, string(mode)), rawPaths)
	if parser.err != nil {
		return
	}
	projection, err := definePropertyProjection(mode, paths)
	if err != nil {
		parser.invalid(joinMetaPath(path, string(mode)), err)
		return
	}
	parser.resources[resource] = projection
}

func (parser *requestMetaParser) parsePropertyPaths(
	path string,
	raw json.RawMessage,
) []string {
	var paths []string
	if err := json.Unmarshal(raw, &paths); err != nil {
		parser.typeMismatch(path, "an array of strings", err)
		return nil
	}
	if len(paths) == 0 {
		parser.err = param.NewError(
			path,
			param.CodeTooSmall,
			fmt.Sprintf("`%s` must contain at least one property path", path),
			nil,
		)
		return nil
	}
	return paths
}

func (parser *requestMetaParser) decodeObject(
	path string,
	raw json.RawMessage,
) map[string]json.RawMessage {
	if parser.err != nil {
		return nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		parser.typeMismatch(path, "an object", err)
		return nil
	}
	return object
}

func (parser *requestMetaParser) attach() {
	if parser.err != nil || parser.req == nil {
		return
	}
	parser.req.meta = cloneMeta(parser.meta)
	parser.req.metaPrepared = true
}

func (parser *requestMetaParser) stripMeta() {
	if parser.err != nil || parser.req == nil || parser.envelope == nil || len(parser.rawMeta) == 0 {
		return
	}
	delete(parser.envelope, requestMetaName)
	body, err := json.Marshal(parser.envelope)
	if err != nil {
		parser.err = param.NewError(
			"request_body",
			param.CodeInvalidBody,
			"request body could not be normalized",
			err,
		)
		return
	}
	parser.req.Body = body
}

func (parser *requestMetaParser) typeMismatch(path string, expected string, cause error) {
	parser.err = param.NewError(
		path,
		param.CodeTypeMismatch,
		fmt.Sprintf("`%s` must be %s", path, expected),
		cause,
	)
}

func (parser *requestMetaParser) invalid(path string, cause error) {
	parser.err = param.NewError(
		path,
		param.CodeInvalid,
		fmt.Sprintf("`%s` is invalid", path),
		cause,
	)
}

func cloneMeta(meta Meta) Meta {
	cloned := Meta{IdempotencyKey: meta.IdempotencyKey}
	if meta.Response != nil {
		cloned.Response = &ResponseMeta{
			Projection: cloneResponseProjection(meta.Response.Projection),
		}
	}
	return cloned
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func joinMetaPath(parent string, child string) string {
	if parent == "" {
		return child
	}
	return parent + "." + child
}

func projectionPath() string {
	return joinMetaPath(joinMetaPath(requestMetaName, responseMetaName), projectionMetaName)
}
