package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/inttegro/httpapi/param"
	"github.com/inttegro/httpapi/response"
)

type projectedLookupParams struct {
	ID string
}

type projectedMediaResponse struct {
	Gallery []string
}

type projectedProductResponse struct {
	ID    string
	Name  string
	Media projectedMediaResponse
}

var projectedMediaResponseShape = response.Object[projectedMediaResponse](
	response.Required("gallery", response.Array[string](), func(media projectedMediaResponse) []string {
		return media.Gallery
	}),
)

var projectedProductResponseShape = response.Object[projectedProductResponse](
	response.Required("id", response.String(), func(product projectedProductResponse) string {
		return product.ID
	}),
	response.Required("name", response.String(), func(product projectedProductResponse) string {
		return product.Name
	}),
	response.Required("media", projectedMediaResponseShape, func(product projectedProductResponse) projectedMediaResponse {
		return product.Media
	}),
)

var projectedLookupResponseShape = response.Envelope(
	response.RequiredField("product", projectedProductResponseShape),
)

func projectedLookupResponses() []ResponseContract {
	return []ResponseContract{
		ResponseBody(http.StatusOK, "Product.", projectedLookupResponseShape),
	}
}

func projectedLookupRequest() *param.Request[projectedLookupParams] {
	return param.JSON[projectedLookupParams]().
		Param(param.Required("id", param.String())).
		Parse(func(values param.Values) (projectedLookupParams, error) {
			return projectedLookupParams{ID: param.Must[string](values, "id")}, nil
		})
}

func TestEndpointPassesResponseProjectionToHandler(t *testing.T) {
	var got projectedLookupParams
	var includesGallery bool
	var includesName bool

	end := DefineJSONEndpoint(JSONEndpointSpec[projectedLookupParams]{
		Method:  POST,
		Path:    "/purchase_intents/lookup",
		Request: projectedLookupRequest(),
		Respond: func(r *Req, params projectedLookupParams) *response.Res {
			got = params
			projection := r.ResponseProjection()
			includesGallery = projection.Includes("product", "media.gallery")
			includesName = projection.Includes("product", "name")
			return response.JSON(http.StatusOK, map[string]string{"id": params.ID})
		},
		Responses: projectedLookupResponses(),
	})

	req := httptest.NewRequest(POST, "/purchase_intents/lookup", strings.NewReader(`{
		"id":"sale_123",
		"request_meta":{"response":{"projection":{
			"product":{"omit":["media.gallery"]}
		}}}
	}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	end.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if got.ID != "sale_123" {
		t.Fatalf("parsed id = %q", got.ID)
	}
	if includesGallery {
		t.Fatal("handler projection includes omitted gallery")
	}
	if !includesName {
		t.Fatal("handler projection excludes unmentioned product name")
	}
}

func TestEndpointAppliesResponseProjectionBeforeAttributeExtraction(t *testing.T) {
	var mediaGetterCalls int
	var galleryGetterCalls int

	mediaShape := response.Object[projectedMediaResponse](
		response.Required("gallery", response.Array[string](), func(media projectedMediaResponse) []string {
			galleryGetterCalls++
			return media.Gallery
		}),
	)
	productShape := response.Object[projectedProductResponse](
		response.Required("id", response.String(), func(product projectedProductResponse) string {
			return product.ID
		}),
		response.Required("name", response.String(), func(product projectedProductResponse) string {
			return product.Name
		}),
		response.Required("media", mediaShape, func(product projectedProductResponse) projectedMediaResponse {
			mediaGetterCalls++
			return product.Media
		}),
	)
	envelope := response.Envelope(response.RequiredField("product", productShape))

	end := DefineJSONEndpoint(JSONEndpointSpec[projectedLookupParams]{
		Method:  POST,
		Path:    "/products/lookup",
		Request: projectedLookupRequest(),
		Respond: func(_ *Req, params projectedLookupParams) *response.Res {
			return response.JSON(http.StatusOK, envelope.Body(
				response.Field("product", projectedProductResponse{
					ID:   params.ID,
					Name: "Utility Sneakers",
					Media: projectedMediaResponse{
						Gallery: []string{"https://example.com/gallery.jpg"},
					},
				}),
			))
		},
		Responses: []ResponseContract{
			ResponseBody(http.StatusOK, "Product.", envelope),
		},
	})

	req := httptest.NewRequest(POST, "/products/lookup", strings.NewReader(`{
		"id":"prod_123",
		"request_meta":{"response":{"projection":{
			"product":{"omit":["media"]}
		}}}
	}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	end.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if mediaGetterCalls != 0 || galleryGetterCalls != 0 {
		t.Fatalf(
			"omitted media getters called: media=%d gallery=%d",
			mediaGetterCalls,
			galleryGetterCalls,
		)
	}
	var body map[string]map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	product := body["product"]
	if product["id"] != "prod_123" || product["name"] != "Utility Sneakers" {
		t.Fatalf("product = %#v", product)
	}
	if _, exists := product["media"]; exists {
		t.Fatalf("product includes omitted media: %#v", product)
	}
}

func TestEndpointProjectionSchemaCompilesLazily(t *testing.T) {
	end := DefineJSONEndpoint(JSONEndpointSpec[projectedLookupParams]{
		Method:  POST,
		Path:    "/purchase_intents/lookup",
		Request: projectedLookupRequest(),
		Respond: func(_ *Req, params projectedLookupParams) *response.Res {
			return response.JSON(http.StatusOK, map[string]string{"id": params.ID})
		},
		Responses: projectedLookupResponses(),
	})
	if end.projectionIndex == nil || len(end.projectionIndex.contracts) == 0 {
		t.Fatal("endpoint did not retain lazy response contracts")
	}

	plainReq := httptest.NewRequest(
		POST,
		"/purchase_intents/lookup",
		strings.NewReader(`{"id":"sale_123"}`),
	)
	plainReq.Header.Set(contentTypeHeaderKey, ApplicationJson)
	plainRes := httptest.NewRecorder()
	end.Handler().ServeHTTP(plainRes, plainReq)
	if plainRes.Code != http.StatusOK {
		t.Fatalf("plain status = %d body=%s", plainRes.Code, plainRes.Body.String())
	}
	if len(end.projectionIndex.contracts) == 0 {
		t.Fatal("schema compiled for a request without a projection")
	}

	projectedReq := httptest.NewRequest(
		POST,
		"/purchase_intents/lookup",
		strings.NewReader(`{
			"id":"sale_123",
			"request_meta":{"response":{"projection":{
				"product":{"pick":["id"]}
			}}}
		}`),
	)
	projectedReq.Header.Set(contentTypeHeaderKey, ApplicationJson)
	projectedRes := httptest.NewRecorder()
	end.Handler().ServeHTTP(projectedRes, projectedReq)
	if projectedRes.Code != http.StatusOK {
		t.Fatalf("projected status = %d body=%s", projectedRes.Code, projectedRes.Body.String())
	}
	if end.projectionIndex.contracts != nil {
		t.Fatal("response contracts retained after lazy compilation")
	}
}

func TestEndpointRejectsInvalidResponseProjectionBeforeHandler(t *testing.T) {
	called := false
	end := DefineJSONEndpoint(JSONEndpointSpec[projectedLookupParams]{
		Method:  POST,
		Path:    "/purchase_intents/lookup",
		Request: projectedLookupRequest(),
		Respond: func(*Req, projectedLookupParams) *response.Res {
			called = true
			return response.Empty(http.StatusNoContent)
		},
	})

	req := httptest.NewRequest(POST, "/purchase_intents/lookup", strings.NewReader(`{
		"id":"sale_123",
		"request_meta":{"response":{"projection":{
			"product":{"pick":["id"],"omit":["media"]}
		}}}
	}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	end.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if called {
		t.Fatal("handler was called for invalid projection")
	}
}

func TestEndpointHandlerReturnsInvalidResponseProjection(t *testing.T) {
	called := false
	end := DefineJSONEndpoint(JSONEndpointSpec[projectedLookupParams]{
		Method:  POST,
		Path:    "/purchase_intents/lookup",
		Request: projectedLookupRequest(),
		Respond: func(_ *Req, _ projectedLookupParams) *response.Res {
			called = true
			return response.Empty(http.StatusNoContent)
		},
		Responses: projectedLookupResponses(),
	})

	req := httptest.NewRequest(POST, "/purchase_intents/lookup", strings.NewReader(`{
		"id":"sale_123",
		"request_meta":{"response":{"projection":{
			"product":{"pick":["id","media.video"]}
		}}}
	}`))
	req.Header.Set(contentTypeHeaderKey, ApplicationJson)
	res := httptest.NewRecorder()

	end.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", res.Code, res.Body.String())
	}
	if called {
		t.Fatal("handler was called for a projection outside its response shape")
	}
	var body response.ErrorBody
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Err == nil {
		t.Fatal("response error = nil")
	}
	if body.Err.Code != string(param.CodeInvalidResponseProjection) {
		t.Fatalf("error code = %q", body.Err.Code)
	}
	if body.Err.Detail != "request_meta.response.projection.product.pick[1]" {
		t.Fatalf("error detail = %q", body.Err.Detail)
	}
}

func TestEndpointMergesResponseProjectionIntoDeclaredRequestMeta(t *testing.T) {
	request := param.JSON[string]().
		Param(param.Required("value", param.String())).
		Param(param.Optional(
			"request_meta",
			param.Object[RequestMeta]().
				Param(param.Optional("idempotency_key", param.String())).
				Parse(func(values param.Values) (RequestMeta, error) {
					key, _ := param.Get[string](values, "idempotency_key")
					return RequestMeta{IdempotencyKey: key}, nil
				}),
		)).
		Parse(func(values param.Values) (string, error) {
			return param.Must[string](values, "value"), nil
		})

	end := DefineJSONEndpoint(JSONEndpointSpec[string]{
		Method:  POST,
		Path:    "/values",
		Request: request,
		Respond: func(*Req, string) *response.Res {
			return response.Empty(http.StatusNoContent)
		},
		Idempotency: EndpointIdempotencySpec{Enabled: true},
	})

	contract := end.RequestContract()
	var requestMeta param.ParameterSpec
	for _, parameter := range contract.Body.Parameters {
		if parameter.Name == "request_meta" {
			requestMeta = parameter
			break
		}
	}
	if requestMeta.Name == "" {
		t.Fatal("request_meta missing")
	}
	if len(requestMeta.Shape.Parameters) != 2 {
		t.Fatalf("request_meta parameters = %#v", requestMeta.Shape.Parameters)
	}
	if requestMeta.Shape.Parameters[0].Name != "idempotency_key" ||
		requestMeta.Shape.Parameters[1].Name != "response" {
		t.Fatalf("request_meta parameters = %#v", requestMeta.Shape.Parameters)
	}
}
