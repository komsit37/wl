package source

import (
	"context"

	"github.com/komsit37/wl/pkg/wl/types"
)

// Source loads watchlists from a specification (e.g., filepath, DSN).
type Source interface {
	Load(ctx context.Context, spec any) ([]types.Watchlist, error)
}
