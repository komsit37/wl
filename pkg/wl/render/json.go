package render

import (
	"encoding/json"
	"io"

	"github.com/komsit37/wl/pkg/wl/types"
)

// jsonModel is the output shape for JSONRenderer.
type jsonModel struct {
	Name    string     `json:"name"`
	Columns []string   `json:"columns"`
	Items   []jsonItem `json:"items"`
}

type jsonItem struct {
	Sym    string         `json:"sym"`
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields"`
}

type JSONRenderer struct{}

func NewJSONRenderer() *JSONRenderer { return &JSONRenderer{} }

func (r *JSONRenderer) Render(w io.Writer, lists []types.Watchlist, opts RenderOptions) error {
	out := make([]jsonModel, 0, len(lists))
	for _, l := range lists {
		cols := l.Columns
		if len(opts.Columns) > 0 {
			cols = opts.Columns
		}
		// Build items; expecting raw values already in Item.Fields
		items := make([]jsonItem, 0, len(l.Items))
		for _, it := range l.Items {
			items = append(items, jsonItem{Sym: it.Sym, Name: it.Name, Fields: it.Fields})
		}
		out = append(out, jsonModel{Name: l.Name, Columns: cols, Items: items})
	}
	enc := json.NewEncoder(w)
	if opts.PrettyJSON {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(out)
}
