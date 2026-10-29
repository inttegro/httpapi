package request

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/inttegro/httpapi/param"
)

func TestPrepareMetaAttachesProjectionAndStripsEnvelope(t *testing.T) {
	req := &Req{Body: []byte(`{
		"id":"sale_123",
		"request_meta":{
			"idempotency_key":"lookup-123",
			"trace":"forward-compatible",
			"response":{
				"projection":{
					"product":{"omit":["media.gallery"]},
					"merchant":{"pick":["app_id","name","branding.accent_color"]}
				}
			}
		}
	}`)}

	if err := req.PrepareMeta(); err != nil {
		t.Fatalf("PrepareMeta error = %v", err)
	}
	if got := string(req.Body); got != `{"id":"sale_123"}` {
		t.Fatalf("sanitized body = %s", got)
	}
	if got := req.RequestMeta().IdempotencyKey; got != "lookup-123" {
		t.Fatalf("idempotency key = %q", got)
	}

	projection := req.ResponseProjection()
	if got := projection.Resources(); !reflect.DeepEqual(got, []string{"merchant", "product"}) {
		t.Fatalf("resources = %#v", got)
	}
	if projection.Includes("product", "media.gallery") {
		t.Fatal("product media gallery is included")
	}
	if !projection.Includes("product", "media.hero") {
		t.Fatal("product media hero is omitted")
	}
	if !projection.Includes("merchant", "branding") {
		t.Fatal("picked descendant did not retain its parent")
	}
	if projection.Includes("merchant", "support") {
		t.Fatal("unpicked merchant support is included")
	}
	if !projection.Includes("order", "id") {
		t.Fatal("unmentioned resource did not retain its complete projection")
	}
}

func TestPrepareMetaRejectsPickAndOmitForSameResource(t *testing.T) {
	req := &Req{Body: []byte(`{
		"request_meta":{"response":{"projection":{
			"product":{"pick":["id"],"omit":["media"]}
		}}}
	}`)}

	err := req.PrepareMeta()
	if err == nil {
		t.Fatal("PrepareMeta error = nil")
	}
	if err.Code != param.CodeMutuallyExclusive {
		t.Fatalf("error code = %q", err.Code)
	}
	if req.ResponseProjection().Defined() {
		t.Fatal("invalid projection was attached")
	}
}

func TestPrepareMetaRequiresPickOrOmitForProjectedResource(t *testing.T) {
	req := &Req{Body: []byte(`{
		"request_meta":{"response":{"projection":{"product":{}}}}
	}`)}

	err := req.PrepareMeta()
	if err == nil {
		t.Fatal("PrepareMeta error = nil")
	}
	if err.Code != param.CodeRequiredChoice {
		t.Fatalf("error code = %q, want %q", err.Code, param.CodeRequiredChoice)
	}
	if err.Param != "request_meta.response.projection.product" {
		t.Fatalf("error param = %q", err.Param)
	}
}

func TestPrepareMetaRejectsEmptyAndDuplicatePropertyPathSets(t *testing.T) {
	tests := []struct {
		name string
		body string
		code param.ErrorCode
	}{
		{
			name: "empty",
			body: `{"request_meta":{"response":{"projection":{"product":{"pick":[]}}}}}`,
			code: param.CodeTooSmall,
		},
		{
			name: "duplicate",
			body: `{"request_meta":{"response":{"projection":{"product":{"omit":["media.gallery","media.gallery"]}}}}}`,
			code: param.CodeInvalid,
		},
		{
			name: "invalid path",
			body: `{"request_meta":{"response":{"projection":{"product":{"pick":["media..gallery"]}}}}}`,
			code: param.CodeInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := &Req{Body: []byte(test.body)}
			err := req.PrepareMeta()
			if err == nil {
				t.Fatal("PrepareMeta error = nil")
			}
			if err.Code != test.code {
				t.Fatalf("error code = %q, want %q", err.Code, test.code)
			}
		})
	}
}

func TestPrepareMetaLeavesBodiesWithoutMetadataUnchanged(t *testing.T) {
	body := []byte(`{ "id": "sale_123" }`)
	req := &Req{Body: append([]byte(nil), body...)}

	if err := req.PrepareMeta(); err != nil {
		t.Fatalf("PrepareMeta error = %v", err)
	}
	if !reflect.DeepEqual(req.Body, body) {
		t.Fatalf("body changed from %q to %q", body, req.Body)
	}
}

func TestResponseProjectionReturnsDefensiveCopies(t *testing.T) {
	req := &Req{Body: []byte(`{
		"request_meta":{"response":{"projection":{
			"product":{"pick":["id","media.gallery"]}
		}}}
	}`)}
	if err := req.PrepareMeta(); err != nil {
		t.Fatalf("PrepareMeta error = %v", err)
	}

	selection, ok := req.ResponseProjection().Resource("product")
	if !ok {
		t.Fatal("product projection missing")
	}
	paths := selection.Paths()
	paths[0] = "mutated"

	selection, _ = req.ResponseProjection().Resource("product")
	if got := selection.Paths()[0]; got != "id" {
		t.Fatalf("stored projection was mutated: %q", got)
	}
}

func TestResponseProjectionUsesStandardJSONShape(t *testing.T) {
	product, err := OmitProperties("media.gallery")
	if err != nil {
		t.Fatalf("OmitProperties error = %v", err)
	}
	merchant, err := PickProperties("app_id", "name")
	if err != nil {
		t.Fatalf("PickProperties error = %v", err)
	}
	projection, err := DefineResponseProjection(map[string]PropertyProjection{
		"product":  product,
		"merchant": merchant,
	})
	if err != nil {
		t.Fatalf("DefineResponseProjection error = %v", err)
	}

	encoded, err := json.Marshal(Meta{Response: &ResponseMeta{Projection: projection}})
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	response := got["response"].(map[string]any)
	resources := response["projection"].(map[string]any)
	if _, ok := resources["product"].(map[string]any)["omit"]; !ok {
		t.Fatalf("product projection = %#v", resources["product"])
	}
	if _, ok := resources["merchant"].(map[string]any)["pick"]; !ok {
		t.Fatalf("merchant projection = %#v", resources["merchant"])
	}
}

func TestPrepareMetaIsIdempotent(t *testing.T) {
	req := &Req{Body: []byte(`{
		"id":"sale_123",
		"request_meta":{"response":{"projection":{
			"product":{"omit":["media.gallery"]}
		}}}
	}`)}
	if err := req.PrepareMeta(); err != nil {
		t.Fatalf("first PrepareMeta error = %v", err)
	}
	if err := req.PrepareMeta(); err != nil {
		t.Fatalf("second PrepareMeta error = %v", err)
	}
	if req.ResponseProjection().Includes("product", "media.gallery") {
		t.Fatal("second preparation discarded projection")
	}
}
