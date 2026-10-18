package endpoint

import (
	"net/http"
	"sync"

	"github.com/zebodotdev/httpapi/param"
	requestpkg "github.com/zebodotdev/httpapi/request"
	responsepkg "github.com/zebodotdev/httpapi/response"
)

// lazyResponseProjectionSchema is kept behind a pointer on Endpoint on purpose:
// Endpoint is a value type and is copied while routes are grouped and mounted.
// Sharing this holder guarantees that every copy observes the same sync.Once
// and compiled immutable index.
//
// Most requests do not ask for a projection. Retaining the already-normalized
// contracts until the first projected request avoids paying the traversal and
// trie allocation cost for endpoints that never use the feature. compile drops
// this holder's contract slice after building the index; Endpoint still owns
// its response contracts for OpenAPI and response metadata.
type lazyResponseProjectionSchema struct {
	once      sync.Once
	contracts []ResponseContract
	schema    requestpkg.ResponseProjectionSchema
}

func newLazyResponseProjectionSchema(
	contracts []ResponseContract,
) *lazyResponseProjectionSchema {
	return &lazyResponseProjectionSchema{contracts: contracts}
}

func (index *lazyResponseProjectionSchema) projectionSchema() requestpkg.ResponseProjectionSchema {
	if index == nil {
		return requestpkg.ResponseProjectionSchema{}
	}
	index.once.Do(index.compile)
	return index.schema
}

func (index *lazyResponseProjectionSchema) compile() {
	shapes := make([]responsepkg.ShapeSpec, 0, len(index.contracts))
	for _, contract := range index.contracts {
		// Only typed successful JSON bodies describe properties a caller may
		// request from a normal endpoint response. Error envelopes, non-JSON
		// bodies, and untyped contracts must not expand the projection surface.
		if contract.Status < http.StatusOK || contract.Status >= http.StatusMultipleChoices {
			continue
		}
		if contract.ContentType != ApplicationJson || contract.Body.Type == "" {
			continue
		}
		shapes = append(shapes, contract.Body)
	}
	index.schema = requestpkg.DeriveResponseProjectionSchema(shapes...)
	// Do not make the lazy cache a second long-lived owner of contracts once the
	// immutable index has everything required for request-time validation.
	index.contracts = nil
}

func (e Endpoint) validateResponseProjection(r *Req) *param.Error {
	if r == nil {
		return nil
	}
	projection := r.ResponseProjection()
	// The common path does not touch sync.Once or compile any schema. This keeps
	// projection support effectively free for callers that accept full responses.
	if !projection.Defined() {
		return nil
	}
	return e.projectionIndex.projectionSchema().Validate(
		projection,
		r.ResponseCaller(),
	)
}
