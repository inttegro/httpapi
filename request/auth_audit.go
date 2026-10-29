package request

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	authpkg "github.com/zebodotdev/httpapi/auth"
)

const (
	authTypeUnknown        = authpkg.TypeUnknown
	authAuditTypeNone      = authpkg.AuditTypeNone
	authAuditTypeBearerKey = authpkg.AuditTypeBearerKey
	authAuditTypeEndpoint  = authpkg.AuditTypeEndpoint

	minCredentialFingerprintLength = 32
)

// ErrUnsupportedAuthorizationScheme reports that an Authorization header used a
// scheme httpapi is not configured to accept.
var ErrUnsupportedAuthorizationScheme = authpkg.ErrUnsupportedAuthorizationScheme

// AuthAudit is the authentication and authorization view embedded in request
// audit output.
type AuthAudit = authpkg.Audit

// AuthFailure is an audit-safe description of authentication or authorization
// failure.
type AuthFailure = authpkg.Failure

// CredentialAudit is the audit-safe description of credential presentation.
type CredentialAudit = authpkg.CredentialAudit

// SessionAudit is the redacted representation of a Session used in audits.
type SessionAudit = authpkg.SessionAudit

func (r *Req) recordAuthFailure(authType string, err error) {
	if r == nil || err == nil {
		return
	}

	failure := authFailureFor(authType, err)
	r.AuthFailure = &failure
}

func (r *Req) recordAuthorizationFailure(failure AuthFailure) {
	if r == nil {
		return
	}

	r.AuthorizationFailure = &failure
}

// RecordAuthorizationFailure records an endpoint authorization failure in the
// request audit record.
func (r *Req) RecordAuthorizationFailure(failure AuthFailure) {
	r.recordAuthorizationFailure(failure)
}

func (r Req) authAudit() AuthAudit {
	authenticated := r.Authenticated()
	sessionAuthorized := r.Authorized()
	authorizationFailure := r.AuthorizationFailure
	return AuthAudit{
		Type:                  authAuditType(r),
		Authenticated:         authenticated,
		SessionAuthorized:     sessionAuthorized,
		Authorized:            sessionAuthorized && authorizationFailure == nil,
		Credential:            credentialAudit(r),
		Failure:               firstAuthFailure(r.AuthFailure, authorizationFailure),
		AuthenticationFailure: r.AuthFailure,
		AuthorizationFailure:  authorizationFailure,
	}
}

// AuthAudit returns the audit-safe authentication and authorization view for r.
func (r *Req) AuthAudit() AuthAudit {
	if r == nil {
		return AuthAudit{}
	}
	return r.authAudit()
}

func credentialAudit(r Req) *CredentialAudit {
	if r.Req == nil {
		return nil
	}

	authorization := strings.TrimSpace(r.Authorization())
	if authorization == "" {
		return nil
	}

	source := authHeaderKey
	if strings.TrimSpace(r.Req.Header.Get(fwdAuthHeaderKey)) != "" {
		source = fwdAuthHeaderKey
	}

	scheme, material, hasMaterial := strings.Cut(authorization, " ")
	audit := &CredentialAudit{
		Presented: true,
		Scheme:    strings.ToLower(strings.TrimSpace(scheme)),
		Source:    source,
	}
	material = strings.TrimSpace(material)
	if hasMaterial && len(material) >= minCredentialFingerprintLength {
		sum := sha256.Sum256([]byte(material))
		audit.Fingerprint = "sha256:" + hex.EncodeToString(sum[:])
	}

	return audit
}

func authAuditType(r Req) string {
	if r.Sess != nil && strings.TrimSpace(r.Sess.AuthMode) == SessionAuthModeService {
		return SessionAuthModeService
	}
	if r.Req == nil {
		return authAuditTypeNone
	}

	auth := strings.TrimSpace(r.Authorization())
	if auth == "" {
		return authAuditTypeNone
	}

	prefix, _, _ := strings.Cut(auth, " ")
	schemes := currentAuthorizationSchemes()
	switch strings.ToLower(strings.TrimSpace(prefix)) {
	case strings.ToLower(schemes.Bearer):
		return authAuditTypeBearerKey
	default:
		for _, scheme := range schemes.ServiceAuthorizationSchemes() {
			if strings.EqualFold(prefix, scheme) {
				return SessionAuthModeService
			}
		}
		return authTypeUnknown
	}
}

func authFailureFor(authType string, err error) AuthFailure {
	failure := AuthFailure{
		Type:    authAuditTypeForAuthType(authType),
		Code:    "authentication_failed",
		Message: "authentication failed",
	}
	var provider interface{ AuthenticationFailure() AuthFailure }
	if errors.As(err, &provider) {
		provided := provider.AuthenticationFailure()
		if strings.TrimSpace(provided.Type) == "" {
			provided.Type = failure.Type
		}
		if strings.TrimSpace(provided.Code) == "" {
			provided.Code = failure.Code
		}
		if strings.TrimSpace(provided.Message) == "" {
			provided.Message = failure.Message
		}
		return provided
	}

	switch authType {
	case authTypeBearer:
		if errors.Is(err, ErrNoKeyToken) {
			failure.Code = "bearer_token_invalid"
			failure.Message = "Bearer authorization header is invalid"
			failure.Detail = "the authorization header did not contain a valid bearer token"
			return failure
		}
		if errors.Is(err, ErrAuthenticatorNotConfigured) {
			failure.Code = "authenticator_not_configured"
			failure.Message = "request authenticator is not configured"
			failure.Detail = "the service did not configure an authenticator for bearer authorization"
			return failure
		}

		failure.Code = "bearer_session_failed"
		failure.Message = "Bearer token could not be exchanged for a session"
		failure.Detail = "the authenticator did not return an authorized bearer session"
		return failure
	case authTypeService:
		if errors.Is(err, ErrNoKeyToken) {
			failure.Code = "service_authorization_invalid"
			failure.Message = "Service authorization header is invalid"
			failure.Detail = "the authorization header did not contain a valid service assertion"
			return failure
		}
		if errors.Is(err, ErrAuthenticatorNotConfigured) {
			failure.Code = "authenticator_not_configured"
			failure.Message = "request authenticator is not configured"
			failure.Detail = "the service did not configure an authenticator for service authorization"
			return failure
		}

		failure.Code = "service_session_failed"
		failure.Message = "Service assertion could not be exchanged for a session"
		failure.Detail = "the authenticator did not return an authorized service session"
		return failure
	default:
		if errors.Is(err, ErrUnsupportedAuthorizationScheme) {
			failure.Code = "authorization_scheme_unsupported"
			failure.Message = "authorization scheme is not supported"
			failure.Detail = "the authorization header uses a scheme this service does not accept"
			return failure
		}

		return failure
	}
}

func firstAuthFailure(failures ...*AuthFailure) *AuthFailure {
	for _, failure := range failures {
		if failure != nil {
			return failure
		}
	}
	return nil
}

func authAuditTypeForAuthType(authType string) string {
	switch authType {
	case authTypeBearer:
		return authAuditTypeBearerKey
	case authTypeService:
		return SessionAuthModeService
	default:
		return authTypeUnknown
	}
}

func sessionAudit(s *Session) *SessionAudit {
	return authpkg.SessionAuditFor(s)
}
