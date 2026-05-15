package cpaimport

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewStateRepo,
	NewBootstrapService,
)
