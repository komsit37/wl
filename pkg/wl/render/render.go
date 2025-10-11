package render

import (
	"io"

	"github.com/komsit37/wl/pkg/wl/types"
)

// Renderer renders watchlists to an output writer.
type Renderer interface {
	Render(w io.Writer, lists []types.Watchlist, opts RenderOptions) error
}

type RenderOptions struct {
	Columns    []string
	Color      bool
	PrettyJSON bool
}
