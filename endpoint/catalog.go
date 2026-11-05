package endpoint

// CatalogEntry is the stable operational projection of one mounted API route.
// It deliberately excludes handlers and runtime-only implementation details so
// it can be serialized into an external control-plane index.
type CatalogEntry struct {
	OperationID      string                   `json:"operation_id"`
	RouteOperationID string                   `json:"route_operation_id,omitempty"`
	Method           HttpMethod               `json:"method"`
	Path             string                   `json:"path"`
	CanonicalPath    string                   `json:"canonical_path"`
	Summary          string                   `json:"summary,omitempty"`
	Kind             OperationKind            `json:"kind,omitempty"`
	Priority         EndpointPriority         `json:"priority,omitempty"`
	Internal         bool                     `json:"internal"`
	Authorization    AuthorizationRequirement `json:"authorization"`
	Idempotent       bool                     `json:"idempotent"`
	Alias            bool                     `json:"alias"`
	Controllable     bool                     `json:"controllable"`
	Controls         []EndpointControlSpec    `json:"controls,omitempty"`
}

// Catalog returns a copy of the mounted route catalog in registration order.
// Canonical routes and aliases are separate entries, while OperationID remains
// the canonical global-availability key shared by all of them.
func (mux *Mux) Catalog() []CatalogEntry {
	mounted := mux.MountedEndpoints()
	if len(mounted) == 0 {
		return nil
	}

	canonicalPaths := make(map[string]string, len(mounted))
	for _, route := range mounted {
		if route.Alias == nil {
			canonicalPaths[route.Endpoint.Operation().ID] = route.Path
		}
	}

	entries := make([]CatalogEntry, 0, len(mounted))
	for _, route := range mounted {
		operation := route.Endpoint.Operation()
		entry := CatalogEntry{
			OperationID:   operation.ID,
			Method:        route.Method,
			Path:          route.Path,
			CanonicalPath: canonicalPaths[operation.ID],
			Summary:       operation.Summary,
			Kind:          operation.Kind,
			Priority:      route.Endpoint.Priority(),
			Internal:      route.Endpoint.IsInternal(),
			Authorization: route.Endpoint.Authorization(),
			Idempotent:    route.Endpoint.IsIdempotent(),
			Controllable:  route.Endpoint.GloballyControllable(),
			Controls:      route.Endpoint.Controls(),
		}
		if route.Alias != nil {
			entry.Alias = true
			entry.RouteOperationID = route.Alias.OperationID
		}
		entries = append(entries, entry)
	}
	return entries
}
