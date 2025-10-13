package pipeline

import (
	"context"
	"io"

	"github.com/komsit37/wl/pkg/wl/columns"
	"github.com/komsit37/wl/pkg/wl/filter"
	"github.com/komsit37/wl/pkg/wl/render"
	"github.com/komsit37/wl/pkg/wl/source"
	"github.com/komsit37/wl/pkg/wl/types"
)

type Runner struct {
	Source   source.Source
	Renderer render.Renderer
	Writer   io.Writer
}

type ExecuteOptions struct {
	Columns     []string
	Filter      filter.Filter
	Color       bool
	PrettyJSON  bool
	MaxColWidth int
}

func (r *Runner) Execute(ctx context.Context, spec any, opts ExecuteOptions) error {
	lists, err := r.Source.Load(ctx, spec)
	if err != nil {
		return err
	}

	// Apply filter by list name
	var filt filter.Filter = filter.Always(true)
	if opts.Filter != nil {
		filt = opts.Filter
	}
	filtered := make([]types.Watchlist, 0, len(lists))
	for _, l := range lists {
		if filt.Match(l.Name) {
			filtered = append(filtered, l)
		}
	}
	lists = filtered

	// Compute columns per list, honoring explicit and overrides
	for i, l := range lists {
		var cols []string
		if len(opts.Columns) > 0 {
			cols = columns.Compute(opts.Columns, l.Items)
		} else {
			cols = columns.Compute(l.Columns, l.Items)
		}
		lists[i].Columns = cols
	}

	return r.Renderer.Render(r.Writer, lists, render.RenderOptions{
		Columns:     opts.Columns,
		Color:       opts.Color,
		PrettyJSON:  opts.PrettyJSON,
		MaxColWidth: opts.MaxColWidth,
	})
}
