package request

import authpkg "github.com/inttegro/httpapi/auth"

const (
	defaultBearerAuthorizationScheme  = authpkg.DefaultBearerAuthorizationScheme
	defaultServiceAuthorizationScheme = authpkg.DefaultServiceAuthorizationScheme
)

// AuthorizationSchemes configures how request parsing interprets Authorization
// header scheme prefixes.
type AuthorizationSchemes = authpkg.AuthorizationSchemes

// ConfigureAuthorizationSchemes sets the Authorization header schemes used by
// NewReq when deciding which authenticator mode to invoke.
func ConfigureAuthorizationSchemes(schemes AuthorizationSchemes) func() {
	return authpkg.ConfigureAuthorizationSchemes(schemes)
}

func currentAuthorizationSchemes() AuthorizationSchemes {
	return authpkg.CurrentAuthorizationSchemes()
}

// CurrentAuthorizationSchemes returns a copy of the globally configured
// Authorization header schemes.
func CurrentAuthorizationSchemes() AuthorizationSchemes {
	return currentAuthorizationSchemes()
}

func normalizeAuthorizationSchemes(schemes AuthorizationSchemes) AuthorizationSchemes {
	return authpkg.NormalizeAuthorizationSchemes(schemes)
}

// NormalizeAuthorizationSchemes trims configured schemes, applies defaults, and
// removes duplicate aliases.
func NormalizeAuthorizationSchemes(schemes AuthorizationSchemes) AuthorizationSchemes {
	return normalizeAuthorizationSchemes(schemes)
}
