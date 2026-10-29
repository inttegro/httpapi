package request

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	authpkg "github.com/inttegro/httpapi/auth"
)

const (
	// SessionAuthModeBearer is the request-facing alias for bearer sessions.
	SessionAuthModeBearer = authpkg.SessionModeBearer

	// SessionAuthModeService is the request-facing alias for service sessions.
	SessionAuthModeService = authpkg.SessionModeService
)

var (
	// ErrNoKeyToken reports that a recognized Authorization scheme did not
	// contain usable credential material.
	ErrNoKeyToken = authpkg.ErrNoKeyToken

	// ErrAuthenticatorNotConfigured reports that request authentication was
	// attempted before a concrete Authenticator was installed.
	ErrAuthenticatorNotConfigured = authpkg.ErrAuthenticatorNotConfigured

	// ErrAuthzFailed reports that credentials were parsed but could not produce
	// an authorized session.
	ErrAuthzFailed = authpkg.ErrAuthzFailed
)

type sessionContextKey struct{}

// App identifies the application a session belongs to.
type App = authpkg.App

// Session is the authenticated request session attached to a Req.
type Session = authpkg.Session

// AuthenticationRequest describes the credential envelope parsed from the
// inbound HTTP request and passed to the configured Authenticator.
type AuthenticationRequest = authpkg.AuthenticationRequest

// Authenticator turns a supported Authorization header into a Session.
//
// Implementations own provider-specific credential verification. httpapi only
// parses the request envelope, calls the authenticator, and attaches the
// returned session.
type Authenticator interface {
	// Authenticate verifies the parsed authentication request and returns a
	// usable session.
	Authenticate(context.Context, *Req, AuthenticationRequest) (*Session, error)
}

// AuthenticatorFunc adapts a function to Authenticator.
type AuthenticatorFunc func(context.Context, *Req, AuthenticationRequest) (*Session, error)

// Authenticate calls f(ctx, req, auth).
func (f AuthenticatorFunc) Authenticate(
	ctx context.Context,
	req *Req,
	auth AuthenticationRequest,
) (*Session, error) {
	return f(ctx, req, auth)
}

type noAuthenticator struct{}

// Authenticate fails because no concrete authenticator is configured.
func (noAuthenticator) Authenticate(
	context.Context,
	*Req,
	AuthenticationRequest,
) (*Session, error) {
	return nil, ErrAuthenticatorNotConfigured
}

var authenticatorState = struct {
	sync.RWMutex
	authenticator Authenticator
}{
	authenticator: noAuthenticator{},
}

// ConfigureAuthenticator installs the package-level authenticator.
//
// It returns a restore function for tests and short-lived overrides. Passing nil
// restores the fail-closed authenticator.
func ConfigureAuthenticator(auth Authenticator) func() {
	if auth == nil {
		auth = noAuthenticator{}
	}

	authenticatorState.Lock()
	prev := authenticatorState.authenticator
	authenticatorState.authenticator = auth
	authenticatorState.Unlock()

	return func() {
		authenticatorState.Lock()
		authenticatorState.authenticator = prev
		authenticatorState.Unlock()
	}
}

func currentAuthenticator() Authenticator {
	authenticatorState.RLock()
	defer authenticatorState.RUnlock()
	return authenticatorState.authenticator
}

// ContextWithSession returns a child context carrying a pre-authenticated
// session. Trusted middleware and tests can use it when authentication has
// already happened before the request reaches an Endpoint.
func ContextWithSession(ctx context.Context, session *Session) context.Context {
	if session == nil {
		return ctx
	}

	return context.WithValue(ctx, sessionContextKey{}, session)
}

// SessionFromContext returns a session previously installed by
// ContextWithSession.
func SessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionContextKey{}).(*Session)
	return session
}

// ContextWithAuthenticatedApp returns a context carrying a generic bearer
// session for an application. It is primarily useful in tests.
func ContextWithAuthenticatedApp(ctx context.Context, appID string) context.Context {
	now := time.Now().UTC()
	return ContextWithSession(ctx, &Session{
		ID:          "sess_context",
		App:         App{ID: appID},
		InitiatedAt: now,
		ExpiresAt:   now.Add(30 * time.Minute),
		MultiUse:    true,
		AuthMode:    SessionAuthModeBearer,
	})
}

// Authenticated reports whether a non-nil request has an authorized session.
func (r *Req) Authenticated() bool { return r != nil && r.Sess.Authorized() }

// Authorized reports whether the request session is usable for general
// authenticated work. Endpoint-specific access rules are applied separately by
// endpoint.Endpoint.
func (r *Req) Authorized() bool { return r.Authenticated() }

// AttachSession attaches an authenticated session to the request and updates
// the request's session and application identifiers.
func (r *Req) AttachSession(sess *Session) {
	if r == nil || sess == nil {
		return
	}

	r.Sess = sess
	r.SessID = sess.ID
	if strings.TrimSpace(sess.App.ID) != "" {
		r.AppID = strings.TrimSpace(sess.App.ID)
	}
}

func (r *Req) authenticate(authType string) error {
	auth := strings.TrimSpace(r.Authorization())
	var token string
	var ok bool

	switch authType {
	case authTypeBearer:
		token, ok = authorizationToken(auth, currentAuthorizationSchemes().Bearer)
	case authTypeService:
		ok = serviceAuthorizationValue(auth, currentAuthorizationSchemes())
	default:
		return nil
	}
	if !ok {
		return ErrNoKeyToken
	}

	sess, err := currentAuthenticator().Authenticate(r.Context(), r, AuthenticationRequest{
		Type:          authType,
		Scheme:        authorizationScheme(auth),
		Authorization: auth,
		Token:         token,
		RequestID:     r.ID,
		Method:        r.Method(),
		RequestTarget: requestTarget(r),
		BodySHA256:    requestBodySHA256(r.Body),
		UserAgent:     r.UserAgent(),
		RemoteAddr:    r.RemoteAddr(),
		TraceID:       strings.TrimSpace(r.Req.Header.Get(traceParentHeaderKey)),
	})
	if err != nil {
		return err
	}
	r.AttachSession(sess)
	return nil
}

func authorizationToken(auth, prefix string) (string, bool) {
	parts := strings.SplitN(strings.TrimSpace(auth), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], prefix) {
		return "", false
	}

	token := strings.TrimSpace(parts[1])
	return token, token != ""
}

func authorizationValue(auth, prefix string) bool {
	parts := strings.SplitN(strings.TrimSpace(auth), " ", 2)
	return len(parts) == 2 &&
		strings.EqualFold(parts[0], prefix) &&
		strings.TrimSpace(parts[1]) != ""
}

func serviceAuthorizationValue(auth string, schemes AuthorizationSchemes) bool {
	for _, scheme := range schemes.ServiceAuthorizationSchemes() {
		if authorizationValue(auth, scheme) {
			return true
		}
	}
	return false
}

func authorizationScheme(auth string) string {
	scheme, _, _ := strings.Cut(strings.TrimSpace(auth), " ")
	return strings.TrimSpace(scheme)
}

func requestTarget(r *Req) string {
	if r == nil || r.Req == nil || r.Req.URL == nil {
		return ""
	}
	return r.Req.URL.RequestURI()
}

func requestBodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
