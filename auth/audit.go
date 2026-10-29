package auth

import (
	"strings"
	"time"
)

const (
	// AuditTypeNone records that the request had no usable authorization
	// material.
	AuditTypeNone = "none"

	// AuditTypeBearerKey records that the request attempted bearer-token
	// authentication.
	AuditTypeBearerKey = "bearer_token"

	// AuditTypeEndpoint records an authorization failure produced by endpoint
	// access policy rather than by credential parsing.
	AuditTypeEndpoint = "endpoint"
)

// Audit is the authentication and authorization view embedded in request audit
// output.
type Audit struct {
	// Type identifies the credential family or access layer responsible for the
	// audit outcome.
	Type string `json:"type"`

	// Authenticated reports whether a valid session was attached to the request.
	Authenticated bool `json:"authenticated"`

	// SessionAuthorized reports whether the attached session itself was usable.
	SessionAuthorized bool `json:"session_authorized"`

	// Authorized reports the final auth decision after endpoint access policy is
	// applied.
	Authorized bool `json:"authorized"`

	// Credential records audit-safe facts about the presented Authorization
	// value without retaining credential material.
	Credential *CredentialAudit `json:"credential,omitempty"`

	// Failure is the first authentication or authorization failure, convenient
	// for consumers that display one failure object.
	Failure *Failure `json:"failure,omitempty"`

	// AuthenticationFailure captures failures while parsing or verifying
	// credentials.
	AuthenticationFailure *Failure `json:"authentication_failure,omitempty"`

	// AuthorizationFailure captures failures after authentication, such as an
	// endpoint requiring service credentials.
	AuthorizationFailure *Failure `json:"authorization_failure,omitempty"`
}

// CredentialAudit records non-reversible metadata about a credential presented
// on a request.
type CredentialAudit struct {
	// Presented reports that an Authorization value was present.
	Presented bool `json:"presented"`

	// Scheme is the normalized Authorization scheme.
	Scheme string `json:"scheme,omitempty"`

	// Source is the header that supplied the effective Authorization value.
	Source string `json:"source,omitempty"`

	// Fingerprint is an optional SHA-256 digest of sufficiently long credential
	// material. It is intended only for correlation and is never reversible.
	Fingerprint string `json:"fingerprint,omitempty"`
}

// Failure is an audit-safe description of an authentication or authorization
// failure.
type Failure struct {
	// Type identifies the layer that produced the failure.
	Type string `json:"type"`

	// Code is a stable machine-readable failure code.
	Code string `json:"code"`

	// Message is a concise human-readable failure summary.
	Message string `json:"message"`

	// Detail gives operators context without exposing secrets.
	Detail string `json:"detail,omitempty"`
}

// SessionAudit is the redacted representation of a Session used in request
// audit records.
type SessionAudit struct {
	// ID is the session identifier.
	ID string `json:"id,omitempty"`

	// App is the application attached to the session.
	App App `json:"app,omitzero"`

	// Token is never the raw credential; when present it is the literal string
	// REDACTED.
	Token string `json:"token,omitempty"`

	// TokenPresent records whether the source session carried a token.
	TokenPresent bool `json:"token_present,omitempty"`

	// SecretKeyID identifies the source credential record.
	SecretKeyID string `json:"secret_key_id,omitempty"`

	// InitiatedAt is copied from the source session.
	InitiatedAt time.Time `json:"initiated_at,omitzero"`

	// ExpiresAt is copied from the source session.
	ExpiresAt time.Time `json:"expires_at,omitzero"`

	// MultiUse records whether the source credential is reusable.
	MultiUse bool `json:"multi_use,omitempty"`

	// AuthMode records the normalized session mode.
	AuthMode string `json:"auth_mode,omitempty"`

	// ActorService records the service actor for service sessions.
	ActorService string `json:"actor_service,omitempty"`

	// ActorServiceIdentity records the canonical service identity.
	ActorServiceIdentity string `json:"actor_service_identity,omitempty"`

	// ActorCertFingerprint records the actor certificate fingerprint.
	ActorCertFingerprint string `json:"actor_cert_fingerprint,omitempty"`

	// IssuerCertificateFingerprint records the issuer certificate fingerprint.
	IssuerCertificateFingerprint string `json:"issuer_certificate_fingerprint,omitempty"`

	// ServiceAuthKeyID identifies the service signing key.
	ServiceAuthKeyID string `json:"service_auth_key_id,omitempty"`

	// ServiceAuthSignatureHash records an audit-safe signature hash.
	ServiceAuthSignatureHash string `json:"service_auth_signature_hash,omitempty"`

	// ServiceAuthDecisionID links to the service-auth verification decision.
	ServiceAuthDecisionID string `json:"service_auth_decision_id,omitempty"`

	// ServiceAuthRoute records the route authorized by the service assertion.
	ServiceAuthRoute string `json:"service_auth_route,omitempty"`

	// ServiceAuthAction records the action authorized by the service assertion.
	ServiceAuthAction string `json:"service_auth_action,omitempty"`

	// SubjectUserID records the delegated user, when present.
	SubjectUserID string `json:"subject_user_id,omitempty"`

	// SubjectMemberID records the delegated membership, when present.
	SubjectMemberID string `json:"subject_member_id,omitempty"`

	// OrganizationID records the organization boundary for the session.
	OrganizationID string `json:"organization_id,omitempty"`

	// Audience records the intended audience.
	Audience string `json:"audience,omitempty"`

	// Scopes records the granted scopes.
	Scopes []string `json:"scopes,omitempty"`

	// Intent records optional caller intent.
	Intent string `json:"intent,omitempty"`

	// LifetimeClass records the lifetime label from the session.
	LifetimeClass string `json:"lifetime_class,omitempty"`

	// RequestID records the request associated with the auth decision.
	RequestID string `json:"request_id,omitempty"`

	// TraceID records tracing context associated with the auth decision.
	TraceID string `json:"trace_id,omitempty"`

	// TokenID records the unique token or assertion identifier.
	TokenID string `json:"jti,omitempty"`

	// IdempotencyKey is never the raw key; when present it is the literal string
	// REDACTED.
	IdempotencyKey string `json:"idempotency_key,omitempty"`

	// IdempotencyKeyPresent records whether the source session carried an
	// idempotency key.
	IdempotencyKeyPresent bool `json:"idempotency_key_present,omitempty"`
}

// SessionAuditFor converts a Session into the redacted structure used for
// audits. It returns nil for a nil session.
func SessionAuditFor(s *Session) *SessionAudit {
	if s == nil {
		return nil
	}

	audit := &SessionAudit{
		ID:                           s.ID,
		App:                          s.App,
		SecretKeyID:                  s.SecretKeyID,
		InitiatedAt:                  s.InitiatedAt,
		ExpiresAt:                    s.ExpiresAt,
		MultiUse:                     s.MultiUse,
		AuthMode:                     s.AuthMode,
		ActorService:                 s.ActorService,
		ActorServiceIdentity:         s.ActorServiceIdentity,
		ActorCertFingerprint:         s.ActorCertFingerprint,
		IssuerCertificateFingerprint: s.IssuerCertificateFingerprint,
		ServiceAuthKeyID:             s.ServiceAuthKeyID,
		ServiceAuthSignatureHash:     s.ServiceAuthSignatureHash,
		ServiceAuthDecisionID:        s.ServiceAuthDecisionID,
		ServiceAuthRoute:             s.ServiceAuthRoute,
		ServiceAuthAction:            s.ServiceAuthAction,
		SubjectUserID:                s.SubjectUserID,
		SubjectMemberID:              s.SubjectMemberID,
		OrganizationID:               s.OrganizationID,
		Audience:                     s.Audience,
		Scopes:                       append([]string(nil), s.Scopes...),
		Intent:                       s.Intent,
		LifetimeClass:                s.LifetimeClass,
		RequestID:                    s.RequestID,
		TraceID:                      s.TraceID,
		TokenID:                      s.TokenID,
	}
	if strings.TrimSpace(s.Token) != "" {
		audit.TokenPresent = true
		audit.Token = "REDACTED"
	}
	if strings.TrimSpace(s.IdempotencyKey) != "" {
		audit.IdempotencyKeyPresent = true
		audit.IdempotencyKey = "REDACTED"
	}

	return audit
}
