package server

import (
	"github.com/inttegro/httpapi/endpoint"
	"github.com/inttegro/httpapi/openapi/gcpapigateway"
	"github.com/inttegro/httpapi/openapi/openapi31"
	"github.com/inttegro/httpapi/openapi/spec"
)

// DescribeOpenAPI31 returns a public OpenAPI 3.1 document for the endpoints
// mounted on srv.
//
// Internal endpoints are omitted because public OpenAPI documents describe the
// external API contract. Configure document title, version, public URLs, and
// path prefix through Config.Description.
func (srv Server) DescribeOpenAPI31() (spec.Document, error) {
	description := srv.config.Description
	return openapi31.Transcriber{
		PathPrefix: description.pathPrefix(),
		Info:       description.info(),
		Servers:    description.publicServers(),
	}.TranscribeGroupsDocument(srv.endpointGroups()...)
}

// DescribeGCPAPIGateway returns a Swagger 2.0 document suitable for GCP API
// Gateway from the endpoints mounted on srv.
//
// Gateway transcription includes internal endpoints because gateway documents
// route the whole mounted surface. Configure host, version, path prefix, and
// default backend through Config.Description.
func (srv Server) DescribeGCPAPIGateway() (spec.Document, error) {
	description := srv.config.Description
	return gcpapigateway.Transcriber{
		PathPrefix:     description.pathPrefix(),
		Info:           description.info(),
		Host:           description.gatewayHost(),
		DefaultBackend: description.defaultBackend(),
	}.TranscribeGroupsDocument(srv.endpointGroups()...)
}

func (srv Server) endpointGroups() []endpoint.EndpointGroup {
	if srv.mux == nil {
		return nil
	}

	return srv.mux.Groups()
}
