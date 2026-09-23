package requestcontract

import (
	"testing"

	"github.com/zebodotdev/httpapi/endpoint"
	"github.com/zebodotdev/httpapi/param"
)

func TestRequestFingerprintIncludesAcceptedContentType(t *testing.T) {
	jsonEndpoint := fingerprintEndpoint(endpoint.ApplicationJson)
	multipartEndpoint := fingerprintEndpoint(endpoint.MultipartFormData)

	jsonFingerprint, err := RequestFingerprint(jsonEndpoint)
	if err != nil {
		t.Fatalf("RequestFingerprint JSON: %v", err)
	}
	multipartFingerprint, err := RequestFingerprint(multipartEndpoint)
	if err != nil {
		t.Fatalf("RequestFingerprint multipart: %v", err)
	}
	if jsonFingerprint == multipartFingerprint {
		t.Fatalf("content type did not change fingerprint %q", jsonFingerprint)
	}
}

func TestDeclareDoesNotClaimFixtureExecution(t *testing.T) {
	contract := Declare(fingerprintEndpoint(endpoint.ApplicationJson))
	if contract.Executable() {
		t.Fatal("declared metadata-only contract is executable")
	}
}

func fingerprintEndpoint(contentType endpoint.ContentType) endpoint.Endpoint {
	return endpoint.DefineEndpoint(endpoint.EndpointSpec{
		Method:  endpoint.POST,
		Path:    "/fingerprint",
		Accepts: contentType,
		Handler: func(*endpoint.Req) {},
		Operation: endpoint.OperationSpec{
			ID: "fingerprintRequest",
		},
		Request: endpoint.RequestContract{
			Required: true,
			Body: param.ShapeSpec{
				Type: param.TypeObject,
				Parameters: []param.ParameterSpec{
					{
						Name:     "value",
						Required: true,
						Shape:    param.ShapeSpec{Type: param.TypeString},
					},
				},
			},
		},
	})
}
