package endpoint

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	e "github.com/inttegro/httpapi/erreur"
)

// EndpointLimitsSpec declares runtime request limits for one endpoint.
type EndpointLimitsSpec = LimitsSpec

type endpointLimitsPolicy struct {
	limits EndpointLimitsSpec

	maxRequestBytesInherited bool
}

// WithLimitsSpec applies runtime request limits to an endpoint.
//
// It replaces all limit fields on the endpoint. Use EndpointGroup defaults when
// you want unset endpoint fields to inherit group-level values.
func WithLimitsSpec(spec EndpointLimitsSpec) EndpointOption {
	spec = normalizeEndpointLimitsSpec(spec)
	return func(e *Endpoint) {
		e.limits.limits = spec
		e.limits.maxRequestBytesInherited = false
	}
}

// ConfigureLimitsSpec sets default runtime request limits for endpoints in the
// group.
//
// Endpoint-level limit fields override group defaults field by field.
func (eg *EndpointGroup) ConfigureLimitsSpec(spec EndpointLimitsSpec) {
	eg.Limits = normalizeEndpointLimitsSpec(spec)
	for i := range eg.Endpoints {
		eg.Endpoints[i].mutableLimitsPolicy().inheritDefaults(eg.Limits)
		eg.Endpoints[i] = eg.Endpoints[i].withRebuiltHandler()
	}
}

// LimitsSpec returns the group's normalized default runtime request limits.
func (eg EndpointGroup) LimitsSpec() EndpointLimitsSpec {
	return normalizeEndpointLimitsSpec(eg.Limits)
}

// LimitsSpec returns the endpoint's normalized runtime request limits.
func (e Endpoint) LimitsSpec() EndpointLimitsSpec {
	return e.limitsSpec()
}

func (e Endpoint) limitsSpec() EndpointLimitsSpec {
	return normalizeEndpointLimitsSpec(e.limits.limits)
}

func (e *Endpoint) mutableLimitsPolicy() *endpointLimitsPolicy {
	return &e.limits
}

func (p *endpointLimitsPolicy) inheritDefaults(defaults EndpointLimitsSpec) {
	defaults = normalizeEndpointLimitsSpec(defaults)
	p.limits = normalizeEndpointLimitsSpec(p.limits)

	if p.limits.MaxRequestBytes == 0 || p.maxRequestBytesInherited {
		p.limits.MaxRequestBytes = defaults.MaxRequestBytes
		p.maxRequestBytesInherited = defaults.MaxRequestBytes != 0
	}
}

func normalizeEndpointLimitsSpec(spec EndpointLimitsSpec) EndpointLimitsSpec {
	return NormalizeLimitsSpec(spec)
}

func enforceEndpointRequestLimit(
	w http.ResponseWriter,
	req *http.Request,
	limits EndpointLimitsSpec,
) *e.Error {
	limits = normalizeEndpointLimitsSpec(limits)
	if req == nil || limits.MaxRequestBytes == 0 {
		return nil
	}

	envelopeBytes := endpointRequestEnvelopeBytes(req)
	if envelopeBytes > limits.MaxRequestBytes {
		return e.RequestTooLarge(limits.MaxRequestBytes)
	}

	remainingBodyBytes := limits.MaxRequestBytes - envelopeBytes
	if req.ContentLength > remainingBodyBytes {
		return e.RequestTooLarge(limits.MaxRequestBytes)
	}

	body := req.Body
	if body == nil {
		body = http.NoBody
	}
	req.Body = http.MaxBytesReader(w, body, remainingBodyBytes)
	return nil
}

func endpointRequestTooLargeError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func endpointRequestEnvelopeBytes(req *http.Request) int64 {
	if req == nil {
		return 0
	}

	size := int64(len(req.Method) + 1 + len(requestTargetForLimit(req)) + 1 + len(req.Proto) + 2)
	if req.Host != "" {
		size += requestHeaderLineBytes("Host", req.Host)
	}
	if req.ContentLength >= 0 && req.Header.Get("Content-Length") == "" {
		size += requestHeaderLineBytes("Content-Length", strconv.FormatInt(req.ContentLength, 10))
	}
	if len(req.TransferEncoding) > 0 && req.Header.Get("Transfer-Encoding") == "" {
		size += requestHeaderLineBytes("Transfer-Encoding", strings.Join(req.TransferEncoding, ", "))
	}

	for name, values := range req.Header {
		for _, value := range values {
			size += requestHeaderLineBytes(name, value)
		}
	}

	size += int64(len("\r\n"))
	return size
}

func requestHeaderLineBytes(name, value string) int64 {
	return int64(len(name) + len(": ") + len(value) + len("\r\n"))
}

func requestTargetForLimit(req *http.Request) string {
	if req == nil {
		return ""
	}
	if req.URL != nil {
		if target := req.URL.RequestURI(); target != "" {
			return target
		}
	}
	return req.RequestURI
}
