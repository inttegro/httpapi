package request

import (
	"sync"

	"github.com/inttegro/httpapi/edge"
)

var edgeProviderState = struct {
	sync.RWMutex
	provider edge.Provider
}{}

// ConfigureEdgeProvider installs the optional provider used to extract
// provider-specific edge evidence while parsing requests. Passing nil disables
// edge extraction. The returned function restores the previous provider.
func ConfigureEdgeProvider(provider edge.Provider) func() {
	edgeProviderState.Lock()
	previous := edgeProviderState.provider
	edgeProviderState.provider = provider
	edgeProviderState.Unlock()

	return func() {
		edgeProviderState.Lock()
		edgeProviderState.provider = previous
		edgeProviderState.Unlock()
	}
}

func currentEdgeProvider() edge.Provider {
	edgeProviderState.RLock()
	defer edgeProviderState.RUnlock()
	return edgeProviderState.provider
}
