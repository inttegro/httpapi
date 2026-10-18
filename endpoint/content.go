package endpoint

import (
	"fmt"
	"maps"
	"strings"
)

// Method is a normalized HTTP method accepted by an Endpoint.
type Method = string

// ContentType is a request or response media type string.
type ContentType = string

const (
	// OPTIONS is the HTTP OPTIONS method.
	OPTIONS Method = "OPTIONS"

	// POST is the HTTP POST method.
	POST Method = "POST"

	// GET is the HTTP GET method.
	GET Method = "GET"

	// ApplicationJson is the default JSON media type accepted by endpoints and
	// emitted by response helpers.
	ApplicationJson ContentType = "application/json"

	// ApplicationFormURLEncoded is the standard browser form media type.
	ApplicationFormURLEncoded ContentType = "application/x-www-form-urlencoded"

	// MultipartFormData is the media type used for file uploads and multipart
	// forms.
	MultipartFormData ContentType = "multipart/form-data"

	// TextHTML is the HTML response media type.
	TextHTML ContentType = "text/html"

	// TextPlain is the plain text response media type used by httpapi.
	TextPlain ContentType = "text/plain; charset=utf-8"
)

// NormalizeMethod trims and uppercases a method and returns a supported method.
//
// Endpoint runtime currently supports GET and POST handlers. Unsupported
// values panic so bad endpoint definitions fail during application startup.
func NormalizeMethod(method Method) Method {
	normalized := Method(strings.ToUpper(strings.TrimSpace(string(method))))
	switch normalized {
	case POST, GET:
		return normalized
	default:
		panic(
			"invalid method for this http endpoint." +
				" supported methods are `GET` and `POST`",
		)
	}
}

// NormalizeContentType trims a media type and applies ApplicationJson when it
// is empty.
func NormalizeContentType(contentType ContentType) ContentType {
	contentType = ContentType(strings.TrimSpace(string(contentType)))
	if contentType == "" {
		return ApplicationJson
	}

	return contentType
}

// NormalizeContentTypes returns a de-duplicated ordered content-type list.
//
// The primary type is included first when it is non-empty, or when no
// additional types are provided.
func NormalizeContentTypes(primary ContentType, additional ...ContentType) []ContentType {
	var contentTypes []ContentType
	seen := map[ContentType]bool{}
	if strings.TrimSpace(string(primary)) != "" || len(additional) == 0 {
		contentType := NormalizeContentType(primary)
		contentTypes = append(contentTypes, contentType)
		seen[contentType] = true
	}
	for _, contentType := range additional {
		contentType = NormalizeContentType(contentType)
		if seen[contentType] {
			continue
		}
		seen[contentType] = true
		contentTypes = append(contentTypes, contentType)
	}

	return contentTypes
}

// NormalizeContentTypeSlice returns a normalized copy of a content-type slice.
//
// Empty input returns a single ApplicationJson entry.
func NormalizeContentTypeSlice(contentTypes []ContentType) []ContentType {
	if len(contentTypes) == 0 {
		return []ContentType{ApplicationJson}
	}

	normalized := make([]ContentType, 0, len(contentTypes))
	seen := map[ContentType]bool{}
	for _, contentType := range contentTypes {
		contentType = NormalizeContentType(contentType)
		if seen[contentType] {
			continue
		}
		seen[contentType] = true
		normalized = append(normalized, contentType)
	}
	return normalized
}

// PrimaryContentType returns the first normalized content type or
// ApplicationJson when the slice is empty.
func PrimaryContentType(contentTypes []ContentType) ContentType {
	if len(contentTypes) == 0 {
		return ApplicationJson
	}
	return NormalizeContentType(contentTypes[0])
}

// CloneContentTypes returns a copy of a content-type slice.
func CloneContentTypes(contentTypes []ContentType) []ContentType {
	if len(contentTypes) == 0 {
		return nil
	}
	return append([]ContentType(nil), contentTypes...)
}

// JoinContentTypes formats a normalized content-type list for logs and error
// messages.
func JoinContentTypes(contentTypes []ContentType) string {
	if len(contentTypes) == 0 {
		return string(ApplicationJson)
	}

	parts := make([]string, 0, len(contentTypes))
	for _, contentType := range contentTypes {
		parts = append(parts, string(NormalizeContentType(contentType)))
	}
	return strings.Join(parts, ", ")
}

// CloneAuthKeys returns a copy of endpoint authorization metadata keys.
func CloneAuthKeys(keys map[string]bool) map[string]bool {
	if len(keys) == 0 {
		return nil
	}

	cloned := make(map[string]bool, len(keys))
	maps.Copy(cloned, keys)

	return cloned
}

// ValidateContentType reports whether actual matches one of the expected media
// types. Matching is substring-based so values with parameters, such as
// application/json; charset=utf-8, satisfy the base media type.
func ValidateContentType(actual ContentType, expected []ContentType) error {
	actual = NormalizeContentType(actual)
	expected = NormalizeContentTypeSlice(expected)
	for _, candidate := range expected {
		if strings.Contains(string(actual), string(candidate)) {
			return nil
		}
	}

	return fmt.Errorf("httpapi: content type %q does not match any of %q", actual, JoinContentTypes(expected))
}

func requestAcceptsMeta(actual ContentType, accepted []ContentType) bool {
	if strings.TrimSpace(string(actual)) == "" {
		return PrimaryContentType(accepted) == ApplicationJson
	}
	return strings.Contains(
		strings.ToLower(string(actual)),
		string(ApplicationJson),
	)
}
