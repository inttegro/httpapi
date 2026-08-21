package cost

import (
	"fmt"
	"maps"
	"strings"
	"time"
)

// Provider identifies the platform, vendor, or organization that produced a
// usage unit.
//
// Examples include "aws", "gcp", "commerce", "stripe", "twilio", or a payment
// provider slug. Provider is a label only; httpapi does not interpret it.
type Provider string

// Service identifies the provider service or product family that produced a
// usage unit.
//
// Examples include "dynamodb", "cloud-run", "censor", "sms", "email", or a
// payment-provider product. Service is a label only; httpapi does not interpret
// it.
type Service string

// SKU identifies the metered item within a provider service.
//
// SKU values should be stable enough for downstream pricing or reporting code
// to map them to service-owned policy. They do not have to match a cloud
// provider invoice SKU exactly.
type SKU string

// Unit identifies the quantity unit for one usage observation.
//
// Examples include "request", "byte", "millisecond", "read-request-unit",
// "vcpu-second", "gib-second", "segment", or "recipient". Unit is deliberately
// free-form because cloud and provider meters differ.
type Unit string

// UsageUnit describes one provider-neutral usage observation for an operation.
//
// UsageUnit records the unit consumed by a request or operation, but it does
// not contain price, currency, billing account, discount, invoice, or margin
// information. Services should put pricing policy in their own sinks.
type UsageUnit struct {
	// Provider identifies the platform, vendor, or organization that produced
	// the usage.
	Provider Provider `json:"provider"`

	// Service identifies the provider service or product family that produced
	// the usage.
	Service Service `json:"service"`

	// SKU identifies the metered item within the provider service.
	SKU SKU `json:"sku"`

	// Unit identifies the quantity unit.
	Unit Unit `json:"unit"`

	// Quantity is the exact number of Unit values consumed.
	Quantity Quantity `json:"quantity"`

	// Labels carries optional low-cardinality metadata useful to service-owned
	// cost sinks, such as table, region, route class, or provider operation.
	// Labels must not contain request bodies, authorization headers, raw tokens,
	// API keys, or other secret material.
	Labels map[string]string `json:"labels,omitempty"`

	// ObservedAt is when the usage was observed. Recorder.Record fills this
	// value when callers leave it empty.
	ObservedAt time.Time `json:"observed_at"`
}

// NewUsageUnit returns a UsageUnit with the required identity and quantity
// fields populated.
func NewUsageUnit(
	provider Provider,
	service Service,
	sku SKU,
	unit Unit,
	quantity Quantity,
) UsageUnit {
	return UsageUnit{
		Provider: provider,
		Service:  service,
		SKU:      sku,
		Unit:     unit,
		Quantity: quantity,
	}
}

// WithLabel returns a copy of u with name set to value in Labels.
//
// Label names should be low-cardinality operational attributes. Do not use
// WithLabel for secrets or user-controlled request bodies.
func (u UsageUnit) WithLabel(name, value string) UsageUnit {
	if u.Labels == nil {
		u.Labels = map[string]string{}
	} else {
		u.Labels = cloneLabels(u.Labels)
	}
	u.Labels[name] = value
	return u
}

// Validate returns an error when u is missing required usage identity fields or
// contains an invalid quantity.
func (u UsageUnit) Validate() error {
	if strings.TrimSpace(string(u.Provider)) == "" {
		return fmt.Errorf("httpapi/cost: usage provider is required")
	}
	if strings.TrimSpace(string(u.Service)) == "" {
		return fmt.Errorf("httpapi/cost: usage service is required")
	}
	if strings.TrimSpace(string(u.SKU)) == "" {
		return fmt.Errorf("httpapi/cost: usage sku is required")
	}
	if strings.TrimSpace(string(u.Unit)) == "" {
		return fmt.Errorf("httpapi/cost: usage unit is required")
	}
	if err := u.Quantity.Valid(); err != nil {
		return err
	}
	if u.Quantity.Value() <= 0 {
		return fmt.Errorf("httpapi/cost: usage quantity must be positive")
	}
	for name := range u.Labels {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("httpapi/cost: usage label name is required")
		}
	}
	return nil
}

// NormalizeUsageUnit trims usage identity fields, clones labels, and validates
// the result.
func NormalizeUsageUnit(u UsageUnit) (UsageUnit, error) {
	u.Provider = Provider(strings.TrimSpace(string(u.Provider)))
	u.Service = Service(strings.TrimSpace(string(u.Service)))
	u.SKU = SKU(strings.TrimSpace(string(u.SKU)))
	u.Unit = Unit(strings.TrimSpace(string(u.Unit)))
	u.Labels = normalizeLabels(u.Labels)
	if err := u.Validate(); err != nil {
		return UsageUnit{}, err
	}
	return u, nil
}

func cloneUsageUnit(u UsageUnit) UsageUnit {
	u.Labels = cloneLabels(u.Labels)
	return u
}

func cloneUsageUnits(units []UsageUnit) []UsageUnit {
	if len(units) == 0 {
		return nil
	}
	out := make([]UsageUnit, 0, len(units))
	for _, unit := range units {
		out = append(out, cloneUsageUnit(unit))
	}
	return out
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	maps.Copy(out, labels)
	return out
}

func normalizeLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return out
}
