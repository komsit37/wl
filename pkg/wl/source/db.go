package source

import (
	"context"
	"fmt"

	"github.com/komsit37/wl/pkg/wl/types"
)

// DBSource is a placeholder for a future database-backed source.
// It currently returns a not implemented error.
type DBSource struct {
	DSN    string
	Driver string
}

func (DBSource) Load(ctx context.Context, spec any) ([]types.Watchlist, error) { //nolint:revive
	return nil, fmt.Errorf("db source not implemented")
}
