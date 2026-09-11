package response

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

const (
	// ResponseMetaName is the canonical top-level JSON envelope field for
	// curated response metadata.
	ResponseMetaName = "response_meta"

	responseMetaDebugName     = "debug"
	responseMetaRequestIDName = "request_id"
)

var unsafeMetaKeyFragments = []string{
	"access_token",
	"api_key",
	"authorization",
	"client_secret",
	"cookie",
	"idempotency_key",
	"password",
	"private_key",
	"provider_payload",
	"raw_request",
	"raw_response",
	"refresh_token",
	"request_body",
	"set_cookie",
	"stack_trace",
}

// ResponseMeta is a builder for the response_meta envelope field.
//
// ResponseMeta is intentionally curated response-body metadata, not a raw HTTP
// conversation dump. Keep HTTP status and headers on the native response, and
// avoid storing request bodies, authorization material, idempotency keys,
// provider payloads, stack traces, or secrets in response_meta.
type ResponseMeta struct {
	values       map[string]any
	availability callerpkg.Set
}

// Meta declares the standard response_meta envelope field when passed to
// Envelope, and creates a builder when used with WithMeta.
func Meta() *ResponseMeta {
	return &ResponseMeta{}
}

// AvailableTo restricts which callers may see response_meta when this value is
// used as an Envelope attribute.
func (meta *ResponseMeta) AvailableTo(callers ...callerpkg.Caller) *ResponseMeta {
	if meta == nil {
		panic("httpapi/response: nil response meta")
	}
	meta.availability = callerpkg.AvailableTo(callers...)
	return meta
}

// Set stores one curated response_meta value.
//
// Set panics for empty or obviously unsafe keys. Passing nil deletes the key so
// callers can fluently add optional values without leaking JSON nulls.
func (meta *ResponseMeta) Set(key string, value any) *ResponseMeta {
	if meta == nil {
		panic("httpapi/response: nil response meta")
	}
	key = normalizeMetaKey(key)
	if value == nil {
		delete(meta.values, key)
		return meta
	}
	validateMetaValue(key, value)
	meta.ensureValues()[key] = value
	return meta
}

// RequestID stores a request identifier in response_meta.
//
// Prefer the native X-Request-Id response header for HTTP clients. This helper
// exists for endpoints that intentionally mirror the identifier into body
// metadata.
func (meta *ResponseMeta) RequestID(requestID string) *ResponseMeta {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return meta.Set(responseMetaRequestIDName, nil)
	}
	return meta.Set(responseMetaRequestIDName, requestID)
}

// Debug stores one value under response_meta.debug.
func (meta *ResponseMeta) Debug(key string, value any) *ResponseMeta {
	if meta == nil {
		panic("httpapi/response: nil response meta")
	}
	key = normalizeMetaKey(key)
	if value == nil {
		debug := meta.debugValues(false)
		delete(debug, key)
		if len(debug) == 0 {
			delete(meta.values, responseMetaDebugName)
		}
		return meta
	}
	validateMetaValue(key, value)
	meta.debugValues(true)[key] = value
	return meta
}

// Empty reports whether meta has no values to render.
func (meta *ResponseMeta) Empty() bool {
	return meta == nil || len(meta.values) == 0
}

// Values returns a snapshot of the JSON object rendered as response_meta.
func (meta *ResponseMeta) Values() map[string]any {
	if meta == nil || len(meta.values) == 0 {
		return nil
	}
	return cloneMetaValues(meta.values)
}

// WithMeta supplies a response_meta value to an Envelope body.
func WithMeta(meta *ResponseMeta) EnvelopeField {
	return Field[*ResponseMeta](ResponseMetaName, meta)
}

func (meta *ResponseMeta) attributeName() string {
	return ResponseMetaName
}

func (meta *ResponseMeta) attributeType() reflect.Type {
	return reflect.TypeFor[*ResponseMeta]()
}

func (meta *ResponseMeta) attributeSpec() AttributeSpec {
	if meta == nil {
		panic("httpapi/response: nil response meta")
	}
	return AttributeSpec{
		Name:         ResponseMetaName,
		Availability: cloneCallerSet(meta.availability),
		Shape: ShapeSpec{
			Type:     TypeObject,
			MapValue: &ShapeSpec{Type: TypeAny},
		},
	}
}

func (meta *ResponseMeta) projectEnvelopeAttribute(
	caller callerpkg.Caller,
	values EnvelopeValues,
) (string, any, bool) {
	if meta == nil {
		panic("httpapi/response: nil response meta")
	}
	if !meta.availability.Allows(caller) {
		return "", nil, false
	}

	value, ok := values.value(ResponseMetaName)
	if !ok || envelopeValueNil(value) {
		return "", nil, false
	}
	typed, ok := value.(*ResponseMeta)
	if !ok {
		panic(fmt.Sprintf(
			"httpapi/response: envelope field %q has type %T, want %T",
			ResponseMetaName,
			value,
			*new(*ResponseMeta),
		))
	}
	if typed.Empty() {
		return "", nil, false
	}

	return ResponseMetaName, typed.Values(), true
}

func (meta *ResponseMeta) ensureValues() map[string]any {
	if meta.values == nil {
		meta.values = map[string]any{}
	}
	return meta.values
}

func (meta *ResponseMeta) debugValues(create bool) map[string]any {
	values := meta.ensureValues()
	debug, ok := values[responseMetaDebugName].(map[string]any)
	if ok {
		return debug
	}
	if !create {
		return nil
	}
	debug = map[string]any{}
	values[responseMetaDebugName] = debug
	return debug
}

func normalizeMetaKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		panic("httpapi/response: response meta key is required")
	}
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(key))
	for _, fragment := range unsafeMetaKeyFragments {
		if strings.Contains(normalized, fragment) {
			panic(fmt.Sprintf("httpapi/response: unsafe response meta key %q", key))
		}
	}
	return key
}

func validateMetaValue(key string, value any) {
	switch value.(type) {
	case http.Header, *http.Header, *http.Request, http.Request:
		panic(fmt.Sprintf("httpapi/response: unsafe response meta value for key %q", key))
	case io.Reader:
		panic(fmt.Sprintf("httpapi/response: unsafe response meta stream for key %q", key))
	case []byte:
		panic(fmt.Sprintf("httpapi/response: unsafe response meta bytes for key %q", key))
	}

	validateMetaValueReflect(key, reflect.ValueOf(value))
}

func validateMetaValueReflect(key string, value reflect.Value) {
	if !value.IsValid() {
		return
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return
		}
		validateMetaValue(key, value.Elem().Interface())
	case reflect.Map:
		validateMetaMapValue(key, value)
	case reflect.Slice, reflect.Array:
		validateMetaSliceValue(key, value)
	case reflect.Chan, reflect.Func:
		panic(fmt.Sprintf("httpapi/response: unsafe response meta value for key %q", key))
	}
}

func validateMetaMapValue(parentKey string, values reflect.Value) {
	if values.Type().Key().Kind() != reflect.String {
		panic(fmt.Sprintf("httpapi/response: response meta map key for %q must be a string", parentKey))
	}

	keys := values.MapKeys()
	slices.SortFunc(keys, func(a reflect.Value, b reflect.Value) int {
		return strings.Compare(a.String(), b.String())
	})
	for _, keyValue := range keys {
		key := keyValue.String()
		normalized := normalizeMetaKey(key)
		validateMetaValue(parentKey+"."+normalized, values.MapIndex(keyValue).Interface())
	}
}

func validateMetaSliceValue(key string, values reflect.Value) {
	if values.Type().Elem().Kind() == reflect.Uint8 {
		panic(fmt.Sprintf("httpapi/response: unsafe response meta bytes for key %q", key))
	}
	for i := 0; i < values.Len(); i++ {
		validateMetaValue(key, values.Index(i).Interface())
	}
}

func cloneMetaValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		if nested, ok := value.(map[string]any); ok {
			cloned[key] = cloneMetaValues(nested)
			continue
		}
		cloned[key] = value
	}
	return cloned
}
