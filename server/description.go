package server

import (
	"strings"

	"github.com/inttegro/httpapi/endpoint"
	"github.com/inttegro/httpapi/openapi/spec"
)

// Description is the server-level API contract metadata used by document
// writers.
//
// Description deliberately avoids provider-specific concepts. Target packages
// such as openapi/openapi31 and openapi/gcpapigateway translate these neutral
// values into the fields their output formats require.
type Description struct {
	// Title is the human-readable API name shown in generated documents.
	Title string

	// Description is optional document-level context for generated specs.
	Description string

	// Version is the API or service contract version represented by generated
	// specs. It is required when writing complete OpenAPI or gateway documents.
	Version string

	// TermsOfService is an optional URL for the API terms of service.
	TermsOfService string

	// Contact identifies the team, person, or organization responsible for the
	// API. Leave it zero-valued when the generated document should not include a
	// contact block.
	Contact Contact

	// License describes the generated API document license. Leave it zero-valued
	// when the generated document should not include a license block.
	License License

	// PathPrefix is prepended to every documented route. Use it for external API
	// versions or gateway prefixes such as "/v1"; keep endpoint definitions
	// local to the service.
	PathPrefix string

	// PublicURLs lists the public base URLs clients should use for the API.
	// These become OpenAPI 3 server entries and are intentionally separate from
	// Config.Addr, Host, and Port, which only describe where the process listens.
	PublicURLs []PublicURL

	// GatewayHost is the externally routed host for gateway-style documents that
	// need a host field. It is provider-neutral even though specific writers may
	// map it to a target-specific document field.
	GatewayHost string

	// DefaultBackend is the fallback upstream route target for gateway-style
	// documents. Endpoint RouteSpec.Backend values override this field.
	DefaultBackend endpoint.RouteBackend
}

// Contact identifies the owner of a generated API document.
type Contact struct {
	// Name is the contact display name.
	Name string

	// URL is the contact information URL.
	URL string

	// Email is the contact email address.
	Email string
}

// License describes the license attached to a generated API document.
type License struct {
	// Name is the license name.
	Name string

	// URL is the optional URL for the license text.
	URL string
}

// PublicURL describes one public base URL for a generated API document.
type PublicURL struct {
	// URL is the public base URL clients can call.
	URL string

	// Description is optional human-readable context for this public URL.
	Description string
}

func (description Description) info() spec.Info {
	return spec.NormalizeInfo(spec.Info{
		Title:          strings.TrimSpace(description.Title),
		Description:    strings.TrimSpace(description.Description),
		TermsOfService: strings.TrimSpace(description.TermsOfService),
		Version:        strings.TrimSpace(description.Version),
		Contact:        description.Contact.specContact(),
		License:        description.License.specLicense(),
	})
}

func (description Description) publicServers() []spec.Server {
	if len(description.PublicURLs) == 0 {
		return nil
	}

	servers := make([]spec.Server, 0, len(description.PublicURLs))
	for _, publicURL := range description.PublicURLs {
		server := spec.Server{
			URL:         strings.TrimSpace(publicURL.URL),
			Description: strings.TrimSpace(publicURL.Description),
		}
		if server.URL == "" {
			continue
		}
		servers = append(servers, server)
	}

	return spec.NormalizeServers(servers)
}

func (description Description) gatewayHost() string {
	return strings.TrimSpace(description.GatewayHost)
}

func (description Description) pathPrefix() string {
	return strings.TrimSpace(description.PathPrefix)
}

func (description Description) defaultBackend() endpoint.RouteBackend {
	return endpoint.NormalizeRouteBackend(description.DefaultBackend)
}

func (contact Contact) specContact() *spec.Contact {
	contact.Name = strings.TrimSpace(contact.Name)
	contact.URL = strings.TrimSpace(contact.URL)
	contact.Email = strings.TrimSpace(contact.Email)
	if contact.Name == "" && contact.URL == "" && contact.Email == "" {
		return nil
	}

	return &spec.Contact{
		Name:  contact.Name,
		URL:   contact.URL,
		Email: contact.Email,
	}
}

func (license License) specLicense() *spec.License {
	license.Name = strings.TrimSpace(license.Name)
	license.URL = strings.TrimSpace(license.URL)
	if license.Name == "" && license.URL == "" {
		return nil
	}

	return &spec.License{
		Name: license.Name,
		URL:  license.URL,
	}
}
