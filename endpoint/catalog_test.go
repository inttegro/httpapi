package endpoint

import "testing"

func TestMuxCatalogProjectsCanonicalAndAliasRoutes(t *testing.T) {
	mux := NewMux()
	mux.MustMount(EndpointGroup{
		PathPrefix: "/widgets",
		Priority:   EndpointPriorityHigh,
		Endpoints: []Endpoint{DefineEndpoint(EndpointSpec{
			Method:  GET,
			Path:    "/lookup",
			Aliases: []AliasSpec{{Path: "/get", OperationID: "getWidget"}},
			Handler: noopTranscriptionHandler,
			Operation: OperationSpec{
				ID:      "lookupWidget",
				Summary: "Look up a widget",
				Kind:    OperationKindRead,
			},
		})},
	})

	catalog := mux.Catalog()
	if len(catalog) != 2 {
		t.Fatalf("catalog entries = %d, want 2", len(catalog))
	}
	canonical := catalog[0]
	if canonical.OperationID != "lookupWidget" || canonical.Path != "/widgets/lookup" || canonical.CanonicalPath != "/widgets/lookup" || canonical.Alias {
		t.Fatalf("canonical entry = %#v", canonical)
	}
	if !canonical.Controllable || canonical.Priority != EndpointPriorityHigh {
		t.Fatalf("canonical control metadata = %#v", canonical)
	}
	alias := catalog[1]
	if alias.OperationID != "lookupWidget" || alias.RouteOperationID != "getWidget" || alias.Path != "/widgets/get" || alias.CanonicalPath != "/widgets/lookup" || !alias.Alias {
		t.Fatalf("alias entry = %#v", alias)
	}

	catalog[0].Controls = append(catalog[0].Controls, EndpointControlSpec{Kind: "gate", Key: "mutated"})
	if got := mux.Catalog()[0].Controls; len(got) != 0 {
		t.Fatalf("catalog accessor exposed mutable state: %#v", got)
	}
}
