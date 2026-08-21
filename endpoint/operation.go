package endpoint

import "strings"

// OperationSpec describes one endpoint operation across documentation,
// completion events, and accounting metadata.
type OperationSpec struct {
	// ID is the stable machine-readable operation identifier.
	//
	// Operation IDs are shared by OpenAPI documents, generated docs, completion
	// events, and cost accounting. EndpointGroup defaults intentionally never
	// inherit ID because operation identifiers must be unique.
	ID string `json:"id,omitempty" yaml:"id,omitempty"`

	// Summary is a short human-readable operation summary for generated docs.
	Summary string `json:"summary,omitempty" yaml:"summary,omitempty"`

	// Accounting carries provider-neutral accounting metadata for this operation.
	Accounting AccountingSpec `json:"accounting" yaml:"accounting,omitempty"`
}

type endpointOperationPolicy struct {
	operation        OperationSpec
	summaryInherited bool
	costInherited    bool
}

// WithOperationSpec applies provider-neutral operation metadata to an endpoint.
func WithOperationSpec(spec OperationSpec) EndpointOption {
	spec = normalizeEndpointOperationSpec(spec)
	return func(e *Endpoint) {
		policy := e.mutableOperationPolicy()
		policy.operation = spec
		policy.summaryInherited = false
		policy.costInherited = false
	}
}

// ConfigureOperationSpec sets default operation metadata for endpoints in the
// group.
//
// Endpoint-level operation summaries and accounting settings override the group
// default. Operation.ID is never inherited because it must remain unique for
// OpenAPI, generated docs, completion, and cost accounting.
func (eg *EndpointGroup) ConfigureOperationSpec(spec OperationSpec) {
	eg.Operation = normalizeEndpointOperationSpec(spec)
	for i := range eg.Endpoints {
		eg.Endpoints[i].mutableOperationPolicy().inheritDefaults(eg.Operation)
		eg.Endpoints[i] = eg.Endpoints[i].withRebuiltHandler()
	}
}

// OperationSpec returns the group's normalized default operation metadata.
func (eg EndpointGroup) OperationSpec() OperationSpec {
	return normalizeEndpointOperationSpec(eg.Operation)
}

// Operation returns the endpoint's normalized operation metadata.
func (e Endpoint) Operation() OperationSpec {
	return e.operationSpec()
}

// CostAccountingEnabled reports whether the endpoint emits request cost usage
// in completion cost events.
func (e Endpoint) CostAccountingEnabled() bool {
	return e.operationSpec().Accounting.CostAccountingEnabled()
}

// WithDefaults returns spec with unset inheritable fields filled from defaults.
//
// Summary and Accounting inherit from defaults. ID intentionally does not
// inherit because operation identifiers must be unique.
func (spec OperationSpec) WithDefaults(defaults OperationSpec) OperationSpec {
	defaults = NormalizeOperationSpec(defaults)
	spec = NormalizeOperationSpec(spec)

	if spec.Summary == "" {
		spec.Summary = defaults.Summary
	}
	spec.Accounting = spec.Accounting.WithDefaults(defaults.Accounting)

	return NormalizeOperationSpec(spec)
}

// CostAccountingEnabled reports whether cost accounting is enabled for spec.
func (spec OperationSpec) CostAccountingEnabled() bool {
	return NormalizeOperationSpec(spec).Accounting.CostAccountingEnabled()
}

// NormalizeOperationSpec trims operation metadata and normalizes accounting.
func NormalizeOperationSpec(spec OperationSpec) OperationSpec {
	spec.ID = strings.TrimSpace(spec.ID)
	spec.Summary = strings.TrimSpace(spec.Summary)
	spec.Accounting = NormalizeAccountingSpec(spec.Accounting)
	return spec
}

func (e Endpoint) operationSpec() OperationSpec {
	return normalizeEndpointOperationSpec(e.operation.operation)
}

func (e *Endpoint) mutableOperationPolicy() *endpointOperationPolicy {
	return &e.operation
}

func (p *endpointOperationPolicy) inheritDefaults(defaults OperationSpec) {
	defaults = normalizeEndpointOperationSpec(defaults)
	p.operation = normalizeEndpointOperationSpec(p.operation)

	if p.operation.Summary == "" || p.summaryInherited {
		p.operation.Summary = defaults.Summary
		p.summaryInherited = defaults.Summary != ""
	}
	if p.operation.Accounting.Cost == CostAccountingDefault || p.costInherited {
		p.operation.Accounting.Cost = defaults.Accounting.Cost
		p.costInherited = defaults.Accounting.Cost != CostAccountingDefault
	}
	p.operation = normalizeEndpointOperationSpec(p.operation)
}

func normalizeEndpointOperationSpec(spec OperationSpec) OperationSpec {
	return NormalizeOperationSpec(spec)
}
