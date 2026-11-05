package endpoint

import (
	"context"
	"testing"
	"time"
)

func TestEvaluateEndpointAvailabilityRulesDeniesMatchingScopedSubject(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	evaluation := EndpointAvailabilityEvaluation{
		OperationID: "createWidget",
		Phase:       EndpointAvailabilityPhaseScoped,
		Subject: EndpointAvailabilitySubject{
			Caller:         "dashboard",
			AppID:          "app_123",
			OrganizationID: "org_123",
		},
	}
	decision, found, err := EvaluateEndpointAvailabilityRules(
		context.Background(),
		"widgets",
		"test",
		evaluation,
		now,
		func(_ context.Context, controlKey string, subject EndpointAvailabilityRuleSubject) (*EndpointAvailabilityRule, error) {
			if subject.Type != EndpointAvailabilitySubjectOrganization {
				return nil, nil
			}
			return &EndpointAvailabilityRule{
				ID:          EndpointAvailabilityRuleID(controlKey, subject.Type, subject.ID),
				ControlKey:  controlKey,
				ControlKind: EndpointAvailabilityRuleKind,
				SubjectType: subject.Type,
				SubjectID:   subject.ID,
				Effect:      EndpointAvailabilityEffectDeny,
				ExpiresAt:   now.Add(time.Hour),
			}, nil
		},
	)
	if err != nil || !found || decision.Available || decision.Reason != "organization_enrollment_deny" {
		t.Fatalf("decision = %#v found = %t error = %v", decision, found, err)
	}
}

func TestEvaluateEndpointAvailabilityRulesDefaultsAvailable(t *testing.T) {
	decision, found, err := EvaluateEndpointAvailabilityRules(
		context.Background(),
		"widgets",
		"test",
		EndpointAvailabilityEvaluation{OperationID: "createWidget", Phase: EndpointAvailabilityPhaseScoped},
		time.Now(),
		func(context.Context, string, EndpointAvailabilityRuleSubject) (*EndpointAvailabilityRule, error) {
			return nil, nil
		},
	)
	if err != nil || found || !decision.Available {
		t.Fatalf("decision = %#v found = %t error = %v", decision, found, err)
	}
}

func TestEndpointAvailabilityRuleIDMatchesControlPlaneEnrollmentID(t *testing.T) {
	got := EndpointAvailabilityRuleID(
		"endpoint:upper:addProductPrice",
		EndpointAvailabilitySubjectApp,
		"app_123",
	)
	const want = "control_enrollment_UF64p3yb_4n22Wu5QQ0v5g1nMwjgymGn"
	if got != want {
		t.Fatalf("rule id = %q, want %q", got, want)
	}
}
