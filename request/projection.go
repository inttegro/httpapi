package request

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProjectionMode identifies how a property projection selects response
// properties.
type ProjectionMode string

const (
	// ProjectionModePick includes only the named property paths.
	ProjectionModePick ProjectionMode = "pick"

	// ProjectionModeOmit includes every public property except the named paths.
	ProjectionModeOmit ProjectionMode = "omit"
)

// PropertyProjection is an immutable pick-or-omit selection for one named
// response resource.
//
// Paths use dot notation. Selecting a parent selects its complete subtree;
// selecting a descendant also keeps the ancestors needed to contain it.
type PropertyProjection struct {
	mode  ProjectionMode
	paths []string
}

// PickProperties returns a projection that includes only paths and the
// ancestors needed to contain them.
func PickProperties(paths ...string) (PropertyProjection, error) {
	return definePropertyProjection(ProjectionModePick, paths)
}

// OmitProperties returns a projection that excludes paths and their
// descendants.
func OmitProperties(paths ...string) (PropertyProjection, error) {
	return definePropertyProjection(ProjectionModeOmit, paths)
}

// Mode returns whether this projection picks or omits property paths. The zero
// value means no selection was supplied and therefore includes every property.
func (projection PropertyProjection) Mode() ProjectionMode {
	return projection.mode
}

// Paths returns a copy of the normalized property paths in this projection.
func (projection PropertyProjection) Paths() []string {
	return append([]string(nil), projection.paths...)
}

// Defined reports whether a pick or omit selection was supplied.
func (projection PropertyProjection) Defined() bool {
	return projection.mode != ""
}

// Includes reports whether path is needed by this projection.
//
// For pick projections, ancestors of a picked path are included because they
// are required to contain the selected descendant. For omit projections, an
// omitted parent excludes its complete subtree while ancestors remain present.
func (projection PropertyProjection) Includes(path string) bool {
	path = strings.TrimSpace(path)
	if !projection.Defined() || path == "" {
		return true
	}

	switch projection.mode {
	case ProjectionModePick:
		for _, selected := range projection.paths {
			if pathsOverlap(path, selected) {
				return true
			}
		}
		return false
	case ProjectionModeOmit:
		for _, omitted := range projection.paths {
			if path == omitted || strings.HasPrefix(path, omitted+".") {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// Omits reports whether path is excluded by this projection.
func (projection PropertyProjection) Omits(path string) bool {
	return !projection.Includes(path)
}

// MarshalJSON encodes the projection using the standard pick-or-omit wire
// shape.
func (projection PropertyProjection) MarshalJSON() ([]byte, error) {
	if !projection.Defined() {
		return []byte("{}"), nil
	}
	return json.Marshal(map[ProjectionMode][]string{
		projection.mode: projection.Paths(),
	})
}

// ResponseProjection contains property projections keyed by response resource
// name, such as "product" or "merchant".
//
// A resource absent from the projection keeps its complete public response
// representation.
type ResponseProjection struct {
	resources map[string]PropertyProjection
}

// DefineResponseProjection returns an immutable projection from resource
// selections.
func DefineResponseProjection(
	resources map[string]PropertyProjection,
) (ResponseProjection, error) {
	return defineResponseProjection(resources)
}

// Defined reports whether the request supplied at least one resource
// projection.
func (projection ResponseProjection) Defined() bool {
	return len(projection.resources) > 0
}

// IsZero supports omitzero JSON encoding for an undefined projection.
func (projection ResponseProjection) IsZero() bool {
	return !projection.Defined()
}

// Resources returns the projected resource names in lexical order.
func (projection ResponseProjection) Resources() []string {
	resources := make([]string, 0, len(projection.resources))
	for resource := range projection.resources {
		resources = append(resources, resource)
	}
	sort.Strings(resources)
	return resources
}

// Resource returns the property projection for resource when one was supplied.
func (projection ResponseProjection) Resource(resource string) (PropertyProjection, bool) {
	selected, ok := projection.resources[strings.TrimSpace(resource)]
	if !ok {
		return PropertyProjection{}, false
	}
	return clonePropertyProjection(selected), true
}

// Includes reports whether a handler needs property path for resource.
// Unmentioned resources include every public property.
func (projection ResponseProjection) Includes(resource string, path string) bool {
	selected, ok := projection.Resource(resource)
	if !ok {
		return true
	}
	return selected.Includes(path)
}

// MarshalJSON encodes resource projections using their resource names as
// object properties.
func (projection ResponseProjection) MarshalJSON() ([]byte, error) {
	resources := make(map[string]PropertyProjection, len(projection.resources))
	for resource, selected := range projection.resources {
		resources[resource] = clonePropertyProjection(selected)
	}
	return json.Marshal(resources)
}

func definePropertyProjection(
	mode ProjectionMode,
	paths []string,
) (PropertyProjection, error) {
	if mode != ProjectionModePick && mode != ProjectionModeOmit {
		return PropertyProjection{}, fmt.Errorf("unsupported projection mode %q", mode)
	}
	if len(paths) == 0 {
		return PropertyProjection{}, fmt.Errorf("%s projection requires at least one property path", mode)
	}

	normalized := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if err := validatePropertyPath(path); err != nil {
			return PropertyProjection{}, err
		}
		if _, exists := seen[path]; exists {
			return PropertyProjection{}, fmt.Errorf("duplicate property path %q", path)
		}
		seen[path] = struct{}{}
		normalized = append(normalized, path)
	}

	return PropertyProjection{mode: mode, paths: normalized}, nil
}

func defineResponseProjection(
	resources map[string]PropertyProjection,
) (ResponseProjection, error) {
	if len(resources) == 0 {
		return ResponseProjection{}, nil
	}

	normalized := make(map[string]PropertyProjection, len(resources))
	for resource, projection := range resources {
		resource = strings.TrimSpace(resource)
		if err := validatePropertyName(resource); err != nil {
			return ResponseProjection{}, fmt.Errorf("invalid projection resource %q: %w", resource, err)
		}
		if _, exists := normalized[resource]; exists {
			return ResponseProjection{}, fmt.Errorf("duplicate projection resource %q", resource)
		}
		normalized[resource] = clonePropertyProjection(projection)
	}
	return ResponseProjection{resources: normalized}, nil
}

func cloneResponseProjection(projection ResponseProjection) ResponseProjection {
	cloned, _ := defineResponseProjection(projection.resources)
	return cloned
}

func clonePropertyProjection(projection PropertyProjection) PropertyProjection {
	return PropertyProjection{
		mode:  projection.mode,
		paths: append([]string(nil), projection.paths...),
	}
}

func pathsOverlap(first string, second string) bool {
	return first == second ||
		strings.HasPrefix(first, second+".") ||
		strings.HasPrefix(second, first+".")
}

func validatePropertyPath(path string) error {
	if path == "" {
		return fmt.Errorf("property path is required")
	}
	for _, segment := range strings.Split(path, ".") {
		if err := validatePropertyName(segment); err != nil {
			return fmt.Errorf("invalid property path %q: %w", path, err)
		}
	}
	return nil
}

func validatePropertyName(name string) error {
	if name == "" {
		return fmt.Errorf("property name is required")
	}
	for _, char := range name {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '_' || char == '-' {
			continue
		}
		return fmt.Errorf("property name %q contains unsupported character %q", name, char)
	}
	return nil
}
