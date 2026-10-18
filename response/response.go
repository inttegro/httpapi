package response

import (
	"io"
	"net/http"
	"time"
)

// Content type aliases kept here so response rendering does not depend on the
// root httpapi package.
const (
	// ApplicationJson is the JSON response content type used by RenderJSON and
	// RenderErr.
	ApplicationJson = "application/json"

	// ApplicationOctetStream is the default binary response content type.
	ApplicationOctetStream = "application/octet-stream"

	// TextHTML is the HTML response content type supported by EncodeResponseBody
	// when the response body is a string.
	TextHTML = "text/html; charset=utf-8"

	// TextPlain is the plain text response content type supported by
	// EncodeResponseBody when the response body is a string.
	TextPlain = "text/plain; charset=utf-8"
)

// Res is the response object populated by handlers and written by endpoint
// runtimes.
type Res struct {
	// ContentType is the HTTP Content-Type header value.
	ContentType string `json:"content_type"`

	// Status is the HTTP response status code.
	Status int `json:"status"`

	// SentAt records when the response object was created.
	SentAt time.Time `json:"sent_at"`

	// Header contains additional response headers. WriteResponse also writes
	// httpapi's standard headers.
	Header http.Header `json:"header"`

	// Body is the JSON, text, or HTML body for non-streaming responses.
	Body any `json:"body"`

	// BodyReader streams response bytes directly to the client. When BodyReader
	// is set, Body is ignored by WriteResponse.
	BodyReader io.Reader `json:"-"`
}

// Option mutates a response during construction.
type Option func(*Res)

// WithHeader appends one header value to the response.
func WithHeader(key string, value string) Option {
	return func(res *Res) {
		if res == nil || key == "" {
			return
		}
		res.ensureHeader().Add(key, value)
	}
}

// WithHeaderValues appends all values for one response header.
func WithHeaderValues(key string, values ...string) Option {
	return func(res *Res) {
		if res == nil || key == "" {
			return
		}
		for _, value := range values {
			res.ensureHeader().Add(key, value)
		}
	}
}

// WithHeaders appends every header value from headers to the response.
func WithHeaders(headers http.Header) Option {
	return func(res *Res) {
		if res == nil {
			return
		}
		res.Header = mergeHeader(res.Header, headers)
	}
}

// WithContentType overrides the response content type.
func WithContentType(contentType string) Option {
	return func(res *Res) {
		if res == nil {
			return
		}
		res.ContentType = contentType
	}
}

// WithSentAt overrides the response creation time. It is primarily useful for
// deterministic tests and replayed responses.
func WithSentAt(sentAt time.Time) Option {
	return func(res *Res) {
		if res == nil || sentAt.IsZero() {
			return
		}
		res.SentAt = sentAt
	}
}

// New returns a response with status, content type, body, and options.
func New(status int, contentType string, body any, options ...Option) *Res {
	res := &Res{
		ContentType: contentType,
		Status:      normalizeStatus(status),
		SentAt:      time.Now(),
		Header:      http.Header{},
		Body:        body,
	}
	applyOptions(res, options...)
	return res
}

// JSON returns a JSON response.
func JSON(status int, body any, options ...Option) *Res {
	return New(status, ApplicationJson, body, options...)
}

// Text returns a plain text response.
func Text(status int, body string, options ...Option) *Res {
	return New(status, TextPlain, body, options...)
}

// HTML returns an HTML response.
func HTML(status int, body string, options ...Option) *Res {
	return New(status, TextHTML, body, options...)
}

// Bytes returns a raw byte response.
func Bytes(status int, contentType string, body []byte, options ...Option) *Res {
	if contentType == "" {
		contentType = ApplicationOctetStream
	}
	return New(status, contentType, append([]byte(nil), body...), options...)
}

// Empty returns a response without a body.
func Empty(status int, options ...Option) *Res {
	res := New(status, "", nil, options...)
	res.Body = nil
	return res
}

// NoContent returns a 204 response without a body.
func NoContent(options ...Option) *Res {
	return Empty(http.StatusNoContent, options...)
}

// Redirect returns an empty response with a Location header.
func Redirect(status int, location string, options ...Option) *Res {
	if status == 0 {
		status = http.StatusFound
	}
	res := Empty(status, options...)
	if location != "" {
		res.ensureHeader().Set("Location", location)
	}
	return res
}

// Stream returns a streaming response without buffering the body in memory.
func Stream(
	status int,
	contentType string,
	header http.Header,
	body io.Reader,
	options ...Option,
) *Res {
	res := New(status, contentType, nil, append([]Option{WithHeaders(header)}, options...)...)
	res.BodyReader = body
	return res
}

// Target is the minimum request surface response renderers need.
type Target interface {
	// SetResponse stores the response that should be written for the current
	// request.
	SetResponse(*Res)
}

// Render sets res on the target.
//
// If res.Body is context-aware and target exposes ResponseContext, Render
// projects the body immediately and stores the projected response on the
// target. This is the ergonomic path for endpoint handlers.
func Render(r Target, res *Res) {
	if r == nil {
		return
	}
	r.SetResponse(responseForTarget(r, res))
}

// RenderJSON sets a JSON response on the target.
//
// Pass response.Body(shape, value) when the JSON body should be filtered through
// a response shape before it is written.
func RenderJSON(r Target, status int, body any, options ...Option) {
	Render(r, JSON(status, body, options...))
}

// RenderObject sets a JSON object response projected for the target's caller.
//
// RenderObject is a convenience wrapper around RenderJSON and Body for the
// common case where the whole response body is one shaped object.
func RenderObject[T any](
	r Target,
	status int,
	shape Shape[T],
	value T,
	options ...Option,
) {
	RenderJSON(r, status, Body(shape, value), options...)
}

// RenderText sets a plain text response on the target.
func RenderText(r Target, status int, body string) {
	Render(r, Text(status, body))
}

// RenderHTML sets an HTML response on the target.
func RenderHTML(r Target, status int, body string) {
	Render(r, HTML(status, body))
}

// RenderBytes sets a raw byte response on the target.
func RenderBytes(r Target, status int, contentType string, body []byte) {
	Render(r, Bytes(status, contentType, body))
}

// RenderNoContent sets a 204 response without a body on the target.
func RenderNoContent(r Target) {
	Render(r, NoContent())
}

// RenderRedirect sets an empty redirect response on the target.
func RenderRedirect(r Target, status int, location string) {
	Render(r, Redirect(status, location))
}

// RenderStream sets a streaming response without buffering the body in memory.
//
// The caller owns the reader until WriteResponse starts writing it; if the
// reader implements io.Closer, WriteResponseStream closes it after copying.
func RenderStream(r Target, status int, contentType string, header http.Header, body io.Reader) {
	Render(r, Stream(status, contentType, header, body))
}

func applyOptions(res *Res, options ...Option) {
	for _, option := range options {
		if option != nil {
			option(res)
		}
	}
}

func (res *Res) ensureHeader() http.Header {
	if res.Header == nil {
		res.Header = http.Header{}
	}
	return res.Header
}

func normalizeStatus(status int) int {
	if status == 0 {
		return http.StatusOK
	}
	return status
}
