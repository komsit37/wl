package render

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"github.com/komsit37/wl/pkg/wl/columns"
	"github.com/komsit37/wl/pkg/wl/enrich"
	"github.com/komsit37/wl/pkg/wl/types"
)

type TableRenderer struct {
	Services columns.Services
}

func NewTableRenderer(s columns.Services) *TableRenderer {
	return &TableRenderer{Services: s}
}

func (r *TableRenderer) Render(w io.Writer, lists []types.Watchlist, opts RenderOptions) error {
	multi := len(lists) > 1
	for li, list := range lists {
		cols := list.Columns

		// Print watchlist name as a standalone line spanning full width
		if multi && strings.TrimSpace(list.Name) != "" {
			fmt.Fprintln(w, text.Bold.Sprint(strings.ToUpper(list.Name)))
		}

		tw := table.NewWriter()
		tw.SetOutputMirror(w)
		tw.SetStyle(table.StyleColoredDark)
		tw.Style().Options.DrawBorder = false
		tw.Style().Options.SeparateRows = false
		tw.Style().Options.SeparateColumns = false

		// Column header row
		hdr := make(table.Row, len(cols))
		for i, c := range cols {
			hdr[i] = strings.ToUpper(c)
		}
		tw.AppendHeader(hdr)

		// Column configs: set by column index for the NAME column if present
		nameIdx := -1
		for i, c := range cols {
			if c == "name" {
				nameIdx = i + 1 // ColumnConfig.Number is 1-based
				break
			}
		}
		nameTransformer := text.Transformer(func(val interface{}) string {
			s := fmt.Sprint(val)
			r := []rune(s)
			if len(r) <= 10 {
				return s
			}
			return string(r[:10])
		})
		if nameIdx > 0 {
			tw.SetColumnConfigs([]table.ColumnConfig{{Number: nameIdx, WidthMax: 10, Align: text.AlignLeft, Transformer: nameTransformer}})
		}

		// Rows
		for _, it := range list.Items {
			row := make(table.Row, len(cols))
			for i, c := range cols {
				val, _ := columns.RenderValue(context.Background(), c, it, r.Services)
				// Colorize for price and chg%
				if opts.Color && (c == "price" || c == "chg%") {
					q, _, _ := r.Services.Quotes.Get(context.Background(), it.Sym, enrich.NeedChgPct|enrich.NeedPrice)
					if q.ChgRaw > 0 {
						val = text.Colors{text.FgGreen}.Sprintf("%s", val)
					} else if q.ChgRaw < 0 {
						val = text.Colors{text.FgRed}.Sprintf("%s", val)
					}
				}
				row[i] = val
			}
			tw.AppendRow(row)
		}

		tw.Render()
		if li < len(lists)-1 {
			// blank line between tables
			fmt.Fprintln(w)
		}
	}
	return nil
}
