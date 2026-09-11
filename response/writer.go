package response

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	callerpkg "github.com/zebodotdev/httpapi/caller"
)

const (
	// HeaderContentType is the canonical Content-Type response header name.
	HeaderContentType = "Content-Type"

	// HeaderRequestID is the standard response header carrying the request id.
	HeaderRequestID = "X-Request-Id"

	// HeaderRequestTiming is the standard response header carrying endpoint
	// write timing.
	HeaderRequestTiming = "X-Request-Timing"

	// HeaderRetryAfter is the standard response header for retry backoff
	// guidance.
	HeaderRetryAfter = "Retry-After"

	contentTypeHeaderKey = "content-type"
	xReqIDHeaderKey      = "x-request-id"
	xReqTimingHeaderKey  = "x-request-timing"
)

var logr = log.New(os.Stdout, "[httpapi/response]: ", log.Flags()|log.Llongfile)

// WriteOptions controls how a Res is written to the HTTP connection.
type WriteOptions struct {
	// RequestID is written as x-request-id.
	RequestID string

	// Caller is used to project caller-aware response bodies before encoding.
	Caller callerpkg.Caller

	// Duration is written as x-request-timing.
	Duration time.Duration

	// WriteTimeout bounds the response write. Zero disables the write deadline.
	WriteTimeout time.Duration
}

// WriteResult describes the completed response write.
type WriteResult struct {
	// Status is the response status code written to the client.
	Status int

	// BytesWritten is the number of bytes written or, for some writers, the
	// encoded body length when the writer reports zero bytes.
	BytesWritten int

	// Streamed reports whether the response was copied from BodyReader.
	Streamed bool
}

// WriteResponse writes a response using httpapi's standard headers, body
// encoding, streaming support, and write deadline handling.
//
// Endpoint runtimes should pass WriteOptions.Caller so shaped JSON bodies are
// projected for the active request caller before encoding. Streaming responses
// are not projected because their bytes are already owned by the application.
func WriteResponse(
	w http.ResponseWriter,
	res *Res,
	opts WriteOptions,
) (WriteResult, error) {
	if w == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response writer is required")
	}
	if res == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response is required")
	}

	if res.BodyReader != nil {
		return WriteResponseStream(w, res, opts)
	}

	body, err := EncodeResponseBodyForCaller(res, opts.Caller)
	if err != nil {
		return WriteResult{}, err
	}

	return WriteResponseBody(w, res, body, opts)
}

// EncodeResponseBody encodes a non-streaming response body.
//
// JSON responses are encoded with encoding/json. TextHTML and TextPlain
// responses use the string body directly when possible. Caller-aware bodies are
// projected with an undefined caller; use EncodeResponseBodyForCaller when
// availability matters.
func EncodeResponseBody(res *Res) ([]byte, error) {
	return EncodeResponseBodyForCaller(res, callerpkg.Caller{})
}

// EncodeResponseBodyForCaller encodes a non-streaming response body after
// projecting caller-aware bodies for caller.
//
// This function is used by endpoint response writing and idempotency capture so
// replay storage records the same caller-visible body that was sent to the
// client.
func EncodeResponseBodyForCaller(res *Res, caller callerpkg.Caller) ([]byte, error) {
	if res == nil {
		return nil, fmt.Errorf("httpapi: response is required")
	}
	if res.BodyReader != nil {
		return nil, fmt.Errorf("httpapi: streamed response body cannot be buffered")
	}
	body := res.Body
	if projected, ok := projectResponseBody(body, caller); ok {
		body = projected
	}
	if body == nil {
		return nil, nil
	}
	mediaType := responseMediaType(res.ContentType)
	switch body := body.(type) {
	case json.RawMessage:
		return append([]byte(nil), body...), nil
	case []byte:
		if !isJSONMediaType(mediaType) {
			return append([]byte(nil), body...), nil
		}
	}
	if strings.HasPrefix(mediaType, "text/") {
		if body, ok := body.(string); ok {
			return []byte(body), nil
		}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func responseMediaType(contentType string) string {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.Split(contentType, ";")[0]
	}
	return strings.ToLower(strings.TrimSpace(mediaType))
}

func isJSONMediaType(mediaType string) bool {
	return mediaType == ApplicationJson || strings.HasSuffix(mediaType, "+json")
}

// WriteResponseBody writes a pre-encoded non-streaming response body.
//
// It applies standard headers, optional write deadlines, and returns the write
// result for audit and logging.
func WriteResponseBody(
	w http.ResponseWriter,
	res *Res,
	body []byte,
	opts WriteOptions,
) (WriteResult, error) {
	if w == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response writer is required")
	}
	if res == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response is required")
	}
	writeDeadlineSet := setEndpointWriteDeadline(w, opts.RequestID, opts.WriteTimeout)
	if writeDeadlineSet {
		defer clearEndpointWriteDeadline(w, opts.RequestID)
	}

	writeResponseHeader(w, res, opts)
	if len(body) == 0 {
		return WriteResult{
			Status: res.Status,
		}, nil
	}

	written, err := w.Write(body)
	if written == 0 {
		written = len(body)
	}

	return WriteResult{
		Status:       res.Status,
		BytesWritten: written,
	}, err
}

// WriteResponseStream writes a streaming response body.
//
// If BodyReader implements io.Closer, it is closed after streaming completes.
func WriteResponseStream(
	w http.ResponseWriter,
	res *Res,
	opts WriteOptions,
) (WriteResult, error) {
	if w == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response writer is required")
	}
	if res == nil {
		return WriteResult{}, fmt.Errorf("httpapi: response is required")
	}
	if res.BodyReader == nil {
		return WriteResult{}, fmt.Errorf("httpapi: streamed response body is required")
	}
	writeDeadlineSet := setEndpointWriteDeadline(w, opts.RequestID, opts.WriteTimeout)
	if writeDeadlineSet {
		defer clearEndpointWriteDeadline(w, opts.RequestID)
	}

	writeResponseHeader(w, res, opts)
	if closer, ok := res.BodyReader.(io.Closer); ok {
		defer closer.Close()
	}
	written, err := io.Copy(w, res.BodyReader)
	return WriteResult{
		Status:       res.Status,
		BytesWritten: int(written),
		Streamed:     true,
	}, err
}

func writeResponseHeader(
	w http.ResponseWriter,
	res *Res,
	opts WriteOptions,
) {
	rsh := w.Header()
	for k, v := range res.Header {
		for _, hv := range v {
			rsh.Add(k, hv)
		}
	}

	if res.ContentType != "" {
		rsh.Set(contentTypeHeaderKey, res.ContentType)
	}
	if opts.Duration != 0 {
		rsh.Set(xReqTimingHeaderKey, opts.Duration.String())
	}
	if opts.RequestID != "" {
		rsh.Set(xReqIDHeaderKey, opts.RequestID)
	}

	res.Status = normalizeStatus(res.Status)
	w.WriteHeader(res.Status)

	res.Header = cloneHeader(rsh)
}

type endpointDeadlineSetter func(*http.ResponseController, time.Time) error

func setEndpointWriteDeadline(
	w http.ResponseWriter,
	requestID string,
	timeout time.Duration,
) bool {
	if timeout < 0 {
		panic("httpapi/response: write timeout cannot be negative")
	}
	if timeout == 0 {
		return false
	}

	return applyEndpointDeadline(
		w,
		requestID,
		time.Now().Add(timeout),
		"write",
		(*http.ResponseController).SetWriteDeadline,
	)
}

func clearEndpointWriteDeadline(w http.ResponseWriter, requestID string) {
	applyEndpointDeadline(
		w,
		requestID,
		time.Time{},
		"write",
		(*http.ResponseController).SetWriteDeadline,
	)
}

func applyEndpointDeadline(
	w http.ResponseWriter,
	requestID string,
	deadline time.Time,
	phase string,
	set endpointDeadlineSetter,
) bool {
	if w == nil {
		return false
	}

	if err := set(http.NewResponseController(w), deadline); err != nil {
		if errors.Is(err, http.ErrNotSupported) {
			return false
		}

		logr.Printf(
			"failed to set endpoint %s deadline:"+
				" request_id=%s deadline=%v error=%v",
			phase,
			requestID,
			deadline,
			err,
		)
		return false
	}

	return true
}
