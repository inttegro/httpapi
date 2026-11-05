package endpoint

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

const (
	EndpointAvailabilitySubjectService      = "service"
	EndpointAvailabilitySubjectCaller       = "caller"
	EndpointAvailabilitySubjectApp          = "app"
	EndpointAvailabilitySubjectOrganization = "organization"

	EndpointAvailabilityRuleKind    = EndpointControlKindCircuitBreaker
	EndpointAvailabilityEffectAllow = "allow"
	EndpointAvailabilityEffectDeny  = "deny"

	endpointAvailabilityControlKeyPrefix = "endpoint:"
)

// EndpointAvailabilityRule is one provider-neutral, subject-scoped endpoint
// breaker enrollment loaded by a service adapter.
type EndpointAvailabilityRule struct {
	ID          string
	ControlKey  string
	ControlKind string
	SubjectType string
	SubjectID   string
	Effect      string
	StartsAt    time.Time
	ExpiresAt   time.Time
	RevokedAt   time.Time
}

// EndpointAvailabilityRuleSubject identifies one lookup performed by a
// service-owned rule loader.
type EndpointAvailabilityRuleSubject struct {
	Type string
	ID   string
}

// EndpointAvailabilityRuleLoader loads one rule without coupling httpapi to a
// database or control-plane provider. A nil rule means no enrollment exists.
type EndpointAvailabilityRuleLoader func(
	context.Context,
	string,
	EndpointAvailabilityRuleSubject,
) (*EndpointAvailabilityRule, error)

// EndpointAvailabilityControlKey returns the stable endpoint-specific breaker
// key shared by service adapters and control planes.
func EndpointAvailabilityControlKey(service, operationID string) string {
	return endpointAvailabilityControlKeyPrefix + strings.TrimSpace(service) + ":" + strings.TrimSpace(operationID)
}

// EndpointAvailabilityRuleID returns the deterministic enrollment identifier
// for one endpoint control and subject.
func EndpointAvailabilityRuleID(controlKey, subjectType, subjectID string) string {
	fingerprint := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(controlKey),
		strings.TrimSpace(subjectType),
		strings.TrimSpace(subjectID),
	}, "\x00")))
	return "control_enrollment_" + base64.RawURLEncoding.EncodeToString(fingerprint[:])[:32]
}

// EvaluateEndpointAvailabilityRules applies default-allow, deny-wins endpoint
// breaker semantics to the subjects available in evaluation.
func EvaluateEndpointAvailabilityRules(
	ctx context.Context,
	service string,
	provider string,
	evaluation EndpointAvailabilityEvaluation,
	now time.Time,
	loader EndpointAvailabilityRuleLoader,
) (EndpointAvailabilityDecision, bool, error) {
	decision := EndpointAvailabilityDecision{Available: true, Provider: strings.TrimSpace(provider)}
	if loader == nil {
		return decision, false, nil
	}

	controlKey := EndpointAvailabilityControlKey(service, evaluation.OperationID)
	subjects := endpointAvailabilityRuleSubjects(service, evaluation)
	type loadResult struct {
		index int
		rule  *EndpointAvailabilityRule
		err   error
	}
	results := make(chan loadResult, len(subjects))
	loaded := 0
	for index, subject := range subjects {
		if strings.TrimSpace(subject.ID) == "" {
			continue
		}
		loaded++
		go func(index int, subject EndpointAvailabilityRuleSubject) {
			rule, err := loader(ctx, controlKey, subject)
			results <- loadResult{index: index, rule: rule, err: err}
		}(index, subject)
	}
	loadedResults := make([]loadResult, len(subjects))
	for range loaded {
		result := <-results
		loadedResults[result.index] = result
	}

	found := false
	var firstLoadErr error
	var deniedDecision *EndpointAvailabilityDecision
	for index, subject := range subjects {
		if strings.TrimSpace(subject.ID) == "" {
			continue
		}
		result := loadedResults[index]
		if result.err != nil {
			if firstLoadErr == nil {
				firstLoadErr = result.err
			}
			continue
		}
		rule := result.rule
		if rule == nil {
			continue
		}
		if err := validateEndpointAvailabilityRule(*rule, controlKey, subject); err != nil {
			return EndpointAvailabilityDecision{}, false, err
		}
		found = true
		if !endpointAvailabilityRuleActive(*rule, now) || strings.TrimSpace(rule.Effect) != EndpointAvailabilityEffectDeny {
			continue
		}
		blocked := EndpointAvailabilityDecision{
			Available: false,
			Provider:  strings.TrimSpace(provider),
			Reason:    subject.Type + "_enrollment_deny",
			ExpiresAt: rule.ExpiresAt,
		}
		if deniedDecision == nil {
			deniedDecision = &blocked
		}
	}
	if deniedDecision != nil {
		return *deniedDecision, true, nil
	}
	if firstLoadErr != nil {
		return EndpointAvailabilityDecision{}, false, firstLoadErr
	}
	return decision, found, nil
}

func endpointAvailabilityRuleSubjects(
	service string,
	evaluation EndpointAvailabilityEvaluation,
) []EndpointAvailabilityRuleSubject {
	if evaluation.Phase != EndpointAvailabilityPhaseScoped {
		return []EndpointAvailabilityRuleSubject{{
			Type: EndpointAvailabilitySubjectService,
			ID:   strings.TrimSpace(service),
		}}
	}
	return []EndpointAvailabilityRuleSubject{
		{Type: EndpointAvailabilitySubjectCaller, ID: strings.TrimSpace(evaluation.Subject.Caller)},
		{Type: EndpointAvailabilitySubjectApp, ID: strings.TrimSpace(evaluation.Subject.AppID)},
		{Type: EndpointAvailabilitySubjectOrganization, ID: strings.TrimSpace(evaluation.Subject.OrganizationID)},
	}
}

func validateEndpointAvailabilityRule(
	rule EndpointAvailabilityRule,
	controlKey string,
	subject EndpointAvailabilityRuleSubject,
) error {
	if strings.TrimSpace(rule.ID) != EndpointAvailabilityRuleID(controlKey, subject.Type, subject.ID) ||
		strings.TrimSpace(rule.ControlKey) != controlKey ||
		strings.TrimSpace(rule.ControlKind) != EndpointAvailabilityRuleKind ||
		strings.TrimSpace(rule.SubjectType) != subject.Type ||
		strings.TrimSpace(rule.SubjectID) != subject.ID ||
		(strings.TrimSpace(rule.Effect) != EndpointAvailabilityEffectAllow &&
			strings.TrimSpace(rule.Effect) != EndpointAvailabilityEffectDeny) {
		return fmt.Errorf(
			"httpapi/endpoint: invalid availability rule for %s/%s:%s",
			controlKey,
			subject.Type,
			subject.ID,
		)
	}
	return nil
}

func endpointAvailabilityRuleActive(rule EndpointAvailabilityRule, now time.Time) bool {
	return (rule.StartsAt.IsZero() || !rule.StartsAt.After(now)) &&
		(rule.ExpiresAt.IsZero() || rule.ExpiresAt.After(now)) &&
		(rule.RevokedAt.IsZero() || rule.RevokedAt.After(now))
}
