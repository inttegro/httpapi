package response_test

import (
	"net/http"
	"testing"

	callerpkg "github.com/inttegro/httpapi/caller"
	requestpkg "github.com/inttegro/httpapi/request"
	responsepkg "github.com/inttegro/httpapi/response"
)

var (
	renderPublicCaller = callerpkg.Define("public-api")
	renderWorkerCaller = callerpkg.Define("worker")
)

type renderRecord struct {
	ID          string
	CreatedFrom string
}

func TestRenderObjectUsesRequestCaller(t *testing.T) {
	shape := renderRecordShape()
	req := &requestpkg.Req{}
	req.AttachCaller(renderWorkerCaller)

	responsepkg.RenderObject(req, http.StatusOK, shape, renderRecord{
		ID:          "rec_123",
		CreatedFrom: "worker",
	})

	body, ok := req.Response().Body.(map[string]any)
	if !ok {
		t.Fatalf("body = %T, want map[string]any", req.Response().Body)
	}
	if body["created_from"] != "worker" {
		t.Fatalf("created_from = %#v, want worker", body["created_from"])
	}
}

func TestRenderJSONProjectsCallerAwareBody(t *testing.T) {
	shape := renderRecordShape()
	req := &requestpkg.Req{}
	req.AttachCaller(renderPublicCaller)

	responsepkg.RenderJSON(req, http.StatusOK, shape.Body(renderRecord{
		ID:          "rec_123",
		CreatedFrom: "worker",
	}))

	body, ok := req.Response().Body.(map[string]any)
	if !ok {
		t.Fatalf("body = %T, want map[string]any", req.Response().Body)
	}
	if _, ok := body["created_from"]; ok {
		t.Fatalf("created_from was visible to public caller: %#v", body)
	}
}

func TestRenderJSONUsesResponseCallerWithoutChangingRequestAuthority(t *testing.T) {
	shape := renderRecordShape()
	req := &requestpkg.Req{}
	req.AttachCaller(renderWorkerCaller)
	policy := callerpkg.DefineProjectionPolicy(callerpkg.Descends(renderWorkerCaller, renderPublicCaller))
	if err := req.ProjectResponseAs(policy, renderPublicCaller); err != nil {
		t.Fatalf("ProjectResponseAs error = %v", err)
	}

	responsepkg.RenderJSON(req, http.StatusOK, shape.Body(renderRecord{
		ID:          "rec_123",
		CreatedFrom: "worker",
	}))

	body, ok := req.Response().Body.(map[string]any)
	if !ok {
		t.Fatalf("body = %T, want map[string]any", req.Response().Body)
	}
	if _, ok := body["created_from"]; ok {
		t.Fatalf("created_from was visible to public response caller: %#v", body)
	}
	if req.RequestCaller() != renderWorkerCaller {
		t.Fatalf("request caller = %q, want %q", req.RequestCaller(), renderWorkerCaller)
	}
}

func renderRecordShape() *responsepkg.ObjectShape[renderRecord] {
	return responsepkg.Object[renderRecord](
		responsepkg.Required("id", responsepkg.String(), func(record renderRecord) string {
			return record.ID
		}),
		responsepkg.Optional("created_from", responsepkg.String(), func(record renderRecord) (string, bool) {
			return record.CreatedFrom, record.CreatedFrom != ""
		}).AvailableTo(renderWorkerCaller),
	)
}
