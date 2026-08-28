package request

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zebodotdev/httpapi/cost"
	"github.com/zebodotdev/httpapi/response"
)

func TestNewReqParsesBodyAndResetsReader(t *testing.T) {
	httpReq := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{"ok":true}`))
	httpReq.Header.Set(contentTypeHeaderKey, "application/json")

	req := NewReq(httpReq)

	if req == nil {
		t.Fatal("NewReq returned nil")
	}
	if string(req.Body) != `{"ok":true}` {
		t.Fatalf("body = %q, want buffered request body", req.Body)
	}

	replay := make([]byte, len(req.Body))
	if _, err := req.Req.Body.Read(replay); err != nil {
		t.Fatalf("reading reset body: %v", err)
	}
	if string(replay) != string(req.Body) {
		t.Fatalf("reset body = %q, want %q", replay, req.Body)
	}
}

func TestNewReqSetsRequestAuditExpiration(t *testing.T) {
	httpReq := httptest.NewRequest(http.MethodPost, "/tasks", strings.NewReader(`{}`))
	startedAt := time.Now().UTC()

	req := NewReq(httpReq)

	if req == nil {
		t.Fatal("NewReq returned nil")
	}
	if req.ExpiresAt.IsZero() {
		t.Fatal("expires_at was not set")
	}
	if req.ExpiresAtUnix != req.ExpiresAt.Unix() {
		t.Fatalf("expires_at_unix = %d, want %d", req.ExpiresAtUnix, req.ExpiresAt.Unix())
	}
	if req.ExpiresAt.Sub(req.RecdAt) != RequestRetention {
		t.Fatalf("expiration window = %v, want %v", req.ExpiresAt.Sub(req.RecdAt), RequestRetention)
	}
	if req.ExpiresAt.Before(startedAt.Add(RequestRetention)) {
		t.Fatalf("expires_at = %v, want at least %v", req.ExpiresAt, startedAt.Add(RequestRetention))
	}

	var audit map[string]any
	if err := json.Unmarshal(mustMarshalReq(t, req), &audit); err != nil {
		t.Fatalf("unmarshal request audit: %v", err)
	}
	if audit["expires_at_unix"] == nil {
		t.Fatalf("request audit omitted expires_at_unix: %#v", audit)
	}
}

func TestNewReqWithErrorReturnsBodyReadError(t *testing.T) {
	want := errors.New("read failed")
	httpReq := httptest.NewRequest(http.MethodPost, "/tasks", nil)
	httpReq.Body = errReadCloser{err: want}

	req, err := NewReqWithError(httpReq)

	if req != nil {
		t.Fatalf("req = %#v, want nil", req)
	}
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestReqImplementsResponseTarget(t *testing.T) {
	req := &Req{}
	res := &response.Res{
		ContentType: response.ApplicationJson,
		Status:      http.StatusAccepted,
		Header:      http.Header{},
		Body:        map[string]bool{"ok": true},
	}

	req.SetResponse(res)

	if req.Response() != res {
		t.Fatal("response target did not retain response")
	}
}

func TestReqRecordsCostUsage(t *testing.T) {
	req := &Req{}
	usage := cost.NewUsageUnit(
		"aws",
		"dynamodb",
		"get-item",
		"read-request-unit",
		cost.Whole(1),
	).WithLabel("table", "orders")

	if err := req.AddCostUsage(usage); err != nil {
		t.Fatalf("AddCostUsage returned error: %v", err)
	}

	usage.Labels["table"] = "mutated"
	units := req.CostUsage()
	if len(units) != 1 {
		t.Fatalf("cost usage units = %d, want 1", len(units))
	}
	if units[0].Labels["table"] != "orders" {
		t.Fatalf("label = %q, want orders", units[0].Labels["table"])
	}

	units[0].Labels["table"] = "changed"
	if got := req.CostUsage()[0].Labels["table"]; got != "orders" {
		t.Fatalf("stored label = %q, want orders", got)
	}
}

func TestNewReqAttachesCostRecorderToContext(t *testing.T) {
	httpReq := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{}`))
	httpReq.Header.Set(traceParentHeaderKey, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	httpReq.Header.Set(xReqIDHeaderKey, "req_external")

	req := NewReq(httpReq)
	if req == nil {
		t.Fatal("NewReq returned nil")
	}

	recorder := cost.RecorderFromContext(req.Context())
	if recorder == nil {
		t.Fatal("request context did not include cost recorder")
	}
	operation := recorder.Operation()
	if operation.ID != req.ID || operation.RootID != req.ID {
		t.Fatalf("operation = %#v, want request id as operation/root id", operation)
	}
	if operation.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("trace id = %q, want traceparent trace id", operation.TraceID)
	}
	if operation.CausationRequestID != "req_external" {
		t.Fatalf("causation request id = %q, want req_external", operation.CausationRequestID)
	}

	if err := cost.Record(req.Context(), cost.NewUsageUnit(
		"aws",
		"dynamodb",
		"query",
		"read-request-unit",
		cost.Whole(2),
	)); err != nil {
		t.Fatalf("cost.Record returned error: %v", err)
	}
	if got := req.CostUsage(); len(got) != 1 || got[0].SKU != "query" {
		t.Fatalf("request cost usage = %#v, want context-recorded query usage", got)
	}
}

type errReadCloser struct {
	err error
}

func (r errReadCloser) Read([]byte) (int, error) {
	return 0, r.err
}

func (r errReadCloser) Close() error {
	return nil
}

var _ io.ReadCloser = errReadCloser{}

func mustMarshalReq(t *testing.T, req *Req) []byte {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return raw
}
