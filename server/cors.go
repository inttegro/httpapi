package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	accessControlAllowCredentialsHeader = "Access-Control-Allow-Credentials"
	accessControlAllowHeadersHeader     = "Access-Control-Allow-Headers"
	accessControlAllowMethodsHeader     = "Access-Control-Allow-Methods"
	accessControlAllowOriginHeader      = "Access-Control-Allow-Origin"
	accessControlExposeHeadersHeader    = "Access-Control-Expose-Headers"
	accessControlMaxAgeHeader           = "Access-Control-Max-Age"
	accessControlRequestHeadersHeader   = "Access-Control-Request-Headers"
	accessControlRequestMethodHeader    = "Access-Control-Request-Method"
	originHeader                        = "Origin"
	varyHeader                          = "Vary"
	wildcardHeaderValue                 = "*"
)

// CORSConfig describes a Cross-Origin Resource Sharing policy.
type CORSConfig struct {
	// AllowedOrigins is the set of origins allowed to read responses. Use "*"
	// only for public resources that do not allow credentials.
	AllowedOrigins []string

	// AllowedMethods is the set of methods accepted in preflight requests. Use
	// "*" to allow every requested method.
	AllowedMethods []string

	// AllowedHeaders is the set of request headers accepted in preflight
	// requests. Use "*" to allow every requested header.
	AllowedHeaders []string

	// ExposeHeaders is emitted on non-preflight responses to expose response
	// headers to browser clients.
	ExposeHeaders []string

	// AllowCredentials emits Access-Control-Allow-Credentials: true. It cannot be
	// combined with a wildcard allowed origin.
	AllowCredentials bool

	// MaxAge is emitted on preflight responses in seconds. Zero omits the header.
	MaxAge time.Duration

	// OptionsStatus is the HTTP status used for allowed preflight responses. Zero
	// uses http.StatusNoContent.
	OptionsStatus int
}

type corsPolicy struct {
	allowedOrigins   []string
	allowedMethods   []string
	allowedHeaders   []string
	exposeHeaders    []string
	allowCredentials bool
	maxAge           time.Duration
	optionsStatus    int
}

type corsResponseWriter struct {
	http.ResponseWriter
	policy  corsPolicy
	request *http.Request
	wrote   bool
}

// PermissiveCORS returns a CORS policy matching the common permissive API
// behavior: every origin, method, and request header is allowed.
func PermissiveCORS() *CORSConfig {
	config := CORSConfig{
		AllowedOrigins: []string{wildcardHeaderValue},
		AllowedMethods: []string{wildcardHeaderValue},
		AllowedHeaders: []string{wildcardHeaderValue},
		OptionsStatus:  http.StatusNoContent,
	}
	return &config
}

// CORS returns middleware that applies config's Cross-Origin Resource Sharing
// policy.
func CORS(config CORSConfig) Middleware {
	return CORSMiddleware(config)
}

// CORSMiddleware returns middleware that applies config's Cross-Origin Resource
// Sharing policy.
func CORSMiddleware(config CORSConfig) Middleware {
	policy := normalizeCORSConfig(config)
	return func(next http.Handler) http.Handler {
		if next == nil {
			next = http.NotFoundHandler()
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r == nil {
				next.ServeHTTP(w, r)
				return
			}
			if isCORSPreflight(r) {
				policy.handlePreflight(w, r)
				return
			}

			next.ServeHTTP(&corsResponseWriter{
				ResponseWriter: w,
				policy:         policy,
				request:        r,
			}, r)
		})
	}
}

func normalizeCORSConfig(config CORSConfig) corsPolicy {
	if config.MaxAge < 0 {
		panic(fmt.Sprintf("httpapi: cors max age cannot be negative: %s", config.MaxAge))
	}
	if config.OptionsStatus != 0 && (config.OptionsStatus < 100 || config.OptionsStatus > 999) {
		panic(fmt.Sprintf("httpapi: cors options status is invalid: %d", config.OptionsStatus))
	}
	if config.AllowCredentials && containsWildcard(config.AllowedOrigins) {
		panic("httpapi: cors wildcard origin cannot allow credentials")
	}

	status := config.OptionsStatus
	if status == 0 {
		status = http.StatusNoContent
	}

	return corsPolicy{
		allowedOrigins:   normalizeCORSList(config.AllowedOrigins, false),
		allowedMethods:   normalizeCORSList(config.AllowedMethods, true),
		allowedHeaders:   normalizeCORSHeaders(config.AllowedHeaders),
		exposeHeaders:    normalizeCORSHeaders(config.ExposeHeaders),
		allowCredentials: config.AllowCredentials,
		maxAge:           config.MaxAge,
		optionsStatus:    status,
	}
}

func (policy corsPolicy) handlePreflight(w http.ResponseWriter, req *http.Request) {
	header := w.Header()
	clearCORSHeaders(header)
	origin, originAllowed, reflectedOrigin := policy.allowedOrigin(req.Header.Get(originHeader))
	methodAllowed := policy.allowedMethod(req.Header.Get(accessControlRequestMethodHeader))
	headersAllowed := policy.allowedRequestHeaders(req.Header.Get(accessControlRequestHeadersHeader))
	if !originAllowed || !methodAllowed || !headersAllowed {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	header.Set(accessControlAllowOriginHeader, origin)
	addVary(header, originHeader, accessControlRequestMethodHeader, accessControlRequestHeadersHeader)
	if reflectedOrigin {
		addVary(header, originHeader)
	}
	if policy.allowCredentials {
		header.Set(accessControlAllowCredentialsHeader, "true")
	}
	if len(policy.allowedMethods) > 0 {
		header.Set(accessControlAllowMethodsHeader, strings.Join(policy.allowedMethods, ", "))
	}
	if len(policy.allowedHeaders) > 0 {
		header.Set(accessControlAllowHeadersHeader, strings.Join(policy.allowedHeaders, ", "))
	}
	if policy.maxAge > 0 {
		header.Set(accessControlMaxAgeHeader, fmt.Sprintf("%.0f", policy.maxAge.Seconds()))
	}
	w.WriteHeader(policy.optionsStatus)
}

func (policy corsPolicy) applyActual(header http.Header, req *http.Request) {
	clearCORSHeaders(header)
	origin, ok, reflected := policy.allowedOrigin(req.Header.Get(originHeader))
	if !ok {
		return
	}

	header.Set(accessControlAllowOriginHeader, origin)
	if reflected {
		addVary(header, originHeader)
	}
	if policy.allowCredentials {
		header.Set(accessControlAllowCredentialsHeader, "true")
	}
	if len(policy.exposeHeaders) > 0 {
		header.Set(accessControlExposeHeadersHeader, strings.Join(policy.exposeHeaders, ", "))
	}
}

func (policy corsPolicy) allowedOrigin(origin string) (string, bool, bool) {
	origin = strings.TrimSpace(origin)
	if origin == "" || len(policy.allowedOrigins) == 0 {
		return "", false, false
	}

	for _, allowed := range policy.allowedOrigins {
		if allowed == wildcardHeaderValue {
			return wildcardHeaderValue, true, false
		}
		if allowed == origin {
			return origin, true, true
		}
	}

	return "", false, false
}

func (policy corsPolicy) allowedMethod(method string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		return false
	}
	for _, allowed := range policy.allowedMethods {
		if allowed == wildcardHeaderValue || allowed == method {
			return true
		}
	}
	return false
}

func (policy corsPolicy) allowedRequestHeaders(rawHeaders string) bool {
	headers := parseCORSHeaderList(rawHeaders)
	if len(headers) == 0 {
		return true
	}
	if containsWildcard(policy.allowedHeaders) {
		return true
	}

	allowed := map[string]bool{}
	for _, header := range policy.allowedHeaders {
		allowed[strings.ToLower(header)] = true
	}
	for _, header := range headers {
		if !allowed[strings.ToLower(header)] {
			return false
		}
	}
	return true
}

func (w *corsResponseWriter) WriteHeader(status int) {
	if !w.wrote {
		w.policy.applyActual(w.Header(), w.request)
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *corsResponseWriter) Write(body []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *corsResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func isCORSPreflight(req *http.Request) bool {
	if req == nil || req.Method != http.MethodOptions {
		return false
	}
	return strings.TrimSpace(req.Header.Get(originHeader)) != "" &&
		strings.TrimSpace(req.Header.Get(accessControlRequestMethodHeader)) != ""
}

func normalizeCORSList(values []string, upper bool) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if upper && value != wildcardHeaderValue {
			value = strings.ToUpper(value)
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeCORSHeaders(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if value != wildcardHeaderValue {
			value = http.CanonicalHeaderKey(value)
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func parseCORSHeaderList(value string) []string {
	parts := strings.Split(value, ",")
	headers := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		headers = append(headers, http.CanonicalHeaderKey(part))
	}
	return headers
}

func containsWildcard(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wildcardHeaderValue {
			return true
		}
	}
	return false
}

func clearCORSHeaders(header http.Header) {
	header.Del(accessControlAllowCredentialsHeader)
	header.Del(accessControlAllowHeadersHeader)
	header.Del(accessControlAllowMethodsHeader)
	header.Del(accessControlAllowOriginHeader)
	header.Del(accessControlExposeHeadersHeader)
	header.Del(accessControlMaxAgeHeader)
}

func addVary(header http.Header, values ...string) {
	if header == nil {
		return
	}

	seen := map[string]bool{}
	for _, existing := range header.Values(varyHeader) {
		for part := range strings.SplitSeq(existing, ",") {
			seen[strings.ToLower(strings.TrimSpace(part))] = true
		}
	}

	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		header.Add(varyHeader, value)
		seen[key] = true
	}
}
