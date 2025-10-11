package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	yfgo "github.com/komsit37/yf-go"
)

// quoteOut holds formatted and raw quote values for rendering.
type quoteOut struct {
	price  string
	chgFmt string
	chgRaw float64
	name   string
}

// QuoteFetcher fetches quotes with a small in-memory cache.
type QuoteFetcher struct {
	client  *yfgo.Client
	cache   map[string]quoteOut
	timeout time.Duration
}

func NewQuoteFetcher(timeout time.Duration) *QuoteFetcher {
	return &QuoteFetcher{
		client:  yfgo.NewClient(),
		cache:   make(map[string]quoteOut),
		timeout: timeout,
	}
}

func (q *QuoteFetcher) Fetch(ctx context.Context, sym string) quoteOut {
	if sym == "" {
		return quoteOut{}
	}
	if v, ok := q.cache[sym]; ok {
		return v
	}
	ctx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()
	res, err := q.client.QuoteSummaryTyped(ctx, sym, []yfgo.QuoteSummaryModule{yfgo.ModulePrice})
	if err != nil || res.Price == nil {
		q.cache[sym] = quoteOut{}
		return quoteOut{}
	}

	// Price
	p := res.Price.RegularMarketPrice
	var priceStr string
	if p.Fmt != "" {
		priceStr = p.Fmt
	} else if p.Raw != nil {
		priceStr = fmt.Sprintf("%.2f", *p.Raw)
	}

	// Change percent
	var chgFmt string
	var chgRaw float64
	cp := res.Price.RegularMarketChangePercent
	if cp.Fmt != "" {
		chgFmt = cp.Fmt
	}
	if cp.Raw != nil {
		chgRaw = *cp.Raw
		if chgFmt == "" {
			chgFmt = fmt.Sprintf("%.2f%%", chgRaw)
		}
	}

	// Name (prefer ShortName, fallback to LongName if available)
	var name string
	if res.Price.ShortName != "" {
		name = res.Price.ShortName
	} else if res.Price.LongName != "" {
		name = res.Price.LongName
	}

	qo := quoteOut{price: priceStr, chgFmt: chgFmt, chgRaw: chgRaw, name: name}
	q.cache[sym] = qo
	return qo
}

// parseWatchlistYAML supports a single, simplified YAML shape:
// - Map with optional columns and required watchlist: "columns: [...]; watchlist: [...]"
// - Nested groups via "name" + "watchlist" inside lists.
// Legacy formats (top-level list, or top-level "items") are no longer supported.
func parseWatchlistYAML(data []byte) (items []map[string]any, columns []string, err error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, err
	}

	// Helper: convert any slice to []string
	toStringSlice := func(v any) []string {
		if v == nil {
			return nil
		}
		switch s := v.(type) {
		case []string:
			return s
		case []any:
			out := make([]string, 0, len(s))
			for _, e := range s {
				if e == nil {
					continue
				}
				out = append(out, fmt.Sprint(e))
			}
			return out
		default:
			return nil
		}
	}

	// Helper: flatten nodes that can be either leaf items or groups with nested watchlists.
	var flatten func(n any, acc *[]map[string]any)
	flatten = func(n any, acc *[]map[string]any) {
		if n == nil {
			return
		}
		switch v := n.(type) {
		case []any:
			for _, e := range v {
				flatten(e, acc)
			}
		case map[string]any:
			// Group if it contains a nested list under "watchlist".
			if wl, ok := v["watchlist"]; ok && wl != nil {
				flatten(wl, acc)
				return
			}
			// Treat as a leaf item.
			*acc = append(*acc, v)
		case map[any]any:
			// Convert to map[string]any if keys are strings.
			m := make(map[string]any, len(v))
			for k, val := range v {
				m[fmt.Sprint(k)] = val
			}
			flatten(m, acc)
		default:
			// Ignore scalars at top-level
		}
	}

	switch r := root.(type) {
	case map[string]any:
		// Extract optional columns
		columns = toStringSlice(r["columns"])
		wl, ok := r["watchlist"]
		if !ok || wl == nil {
			return nil, nil, fmt.Errorf("invalid yaml: expected map with 'watchlist' key")
		}
		flatten(wl, &items)
		return items, columns, nil
	case map[any]any:
		// Convert and recurse
		m := make(map[string]any, len(r))
		for k, v := range r {
			m[fmt.Sprint(k)] = v
		}
		// Re-run the map[string]any branch
		columns = toStringSlice(m["columns"])
		wl, ok := m["watchlist"]
		if !ok || wl == nil {
			return nil, nil, fmt.Errorf("invalid yaml: expected map with 'watchlist' key")
		}
		flatten(wl, &items)
		return items, columns, nil
	default:
		return nil, nil, fmt.Errorf("invalid yaml: expected map with 'watchlist' key")
	}
}

// computeColumns determines the final column order. If explicit is provided,
// it is respected; otherwise keys are discovered across items, preferring
// "sym" first then sorted remainder. Ensures "name" then "price" then "chg%"
// after "sym" when "sym" exists.
func computeColumns(items []map[string]any, explicit []string) []string {
	keys := make([]string, 0, 8)
	if len(explicit) > 0 {
		keys = append(keys, explicit...)
	} else {
		keySet := map[string]struct{}{}
		for _, m := range items {
			for k := range m {
				keySet[k] = struct{}{}
			}
		}
		if _, ok := keySet["sym"]; ok {
			keys = append(keys, "sym")
			delete(keySet, "sym")
		}
		var rest []string
		for k := range keySet {
			rest = append(rest, k)
		}
		sort.Strings(rest)
		keys = append(keys, rest...)
	}

	// Ensure computed columns when sym exists.
	symIdx := -1
	for i, k := range keys {
		if k == "sym" {
			symIdx = i
			break
		}
	}
	if symIdx >= 0 {
		// Ensure name right after sym.
		hasName := false
		for _, k := range keys {
			if k == "name" {
				hasName = true
				break
			}
		}
		if !hasName {
			keys = append(keys, "")
			copy(keys[symIdx+2:], keys[symIdx+1:])
			keys[symIdx+1] = "name"
		}

		// Ensure price right after name (or sym if name absent originally).
		hasPrice := false
		for _, k := range keys {
			if k == "price" {
				hasPrice = true
				break
			}
		}
		if !hasPrice {
			insertAfter := symIdx
			for i, k := range keys {
				if k == "name" {
					insertAfter = i
					break
				}
			}
			keys = append(keys, "")
			copy(keys[insertAfter+2:], keys[insertAfter+1:])
			keys[insertAfter+1] = "price"
		}

		// Ensure chg% after price.
		priceIdx := -1
		for i, k := range keys {
			if k == "price" {
				priceIdx = i
				break
			}
		}
		if priceIdx >= 0 {
			hasChg := false
			for _, k := range keys {
				if k == "chg%" {
					hasChg = true
					break
				}
			}
			if !hasChg {
				keys = append(keys, "")
				copy(keys[priceIdx+2:], keys[priceIdx+1:])
				keys[priceIdx+1] = "chg%"
			}
		}
	}

	return keys
}

// renderTable builds and renders the table to the provided writer.
func renderTable(w io.Writer, items []map[string]any, keys []string, fetcher *QuoteFetcher) {
	tw := table.NewWriter()
	tw.SetOutputMirror(w)
	// Set table style and options
	tw.SetStyle(table.StyleColoredDark)
	tw.Style().Options.DrawBorder = false
	tw.Style().Options.SeparateRows = false
	tw.Style().Options.SeparateColumns = false

	// Header
	hdr := make(table.Row, len(keys))
	for i, k := range keys {
		hdr[i] = strings.ToUpper(k)
	}
	tw.AppendHeader(hdr)

	// Column-specific configs (use header names)
	nameTransformer := text.Transformer(func(val interface{}) string {
		s := fmt.Sprint(val)
		r := []rune(s)
		if len(r) <= 10 {
			return s
		}
		return string(r[:10])
	})
	tw.SetColumnConfigs([]table.ColumnConfig{
		{Name: "NAME", WidthMax: 10, Align: text.AlignLeft, Transformer: nameTransformer},
	})

	// Rows
	for _, m := range items {
		row := make(table.Row, len(keys))
		var symVal string
		if v, ok := m["sym"]; ok && v != nil {
			symVal = fmt.Sprint(v)
		}
		qo := fetcher.Fetch(context.Background(), symVal)
		for i, k := range keys {
			switch k {
			case "price":
				val := qo.price
				if qo.chgRaw > 0 {
					val = text.Colors{text.FgGreen}.Sprintf("%s", val)
				} else if qo.chgRaw < 0 {
					val = text.Colors{text.FgRed}.Sprintf("%s", val)
				}
				row[i] = val
				continue
			case "chg%":
				val := qo.chgFmt
				if qo.chgRaw > 0 {
					val = text.Colors{text.FgGreen}.Sprintf("%s", val)
				} else if qo.chgRaw < 0 {
					val = text.Colors{text.FgRed}.Sprintf("%s", val)
				}
				row[i] = val
				continue
			case "name":
				// Prefer YAML-provided name, else fetched
				var name string
				if v, ok := m["name"]; ok && v != nil {
					name = fmt.Sprint(v)
				} else {
					name = qo.name
				}
				row[i] = name
				continue
			}
			if v, ok := m[k]; ok && v != nil {
				row[i] = fmt.Sprint(v)
			} else {
				row[i] = ""
			}
		}
		tw.AppendRow(row)
	}

	tw.Render()
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "wl <file.yaml>",
		Short: "Render a YAML watchlist to a table",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("requires exactly 1 YAML file argument")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			filename := args[0]
			f, err := os.Open(filename)
			if err != nil {
				return fmt.Errorf("open %s: %w", filename, err)
			}
			defer f.Close()
			cmd.SilenceUsage = true

			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read %s: %w", filename, err)
			}
			items, explicitCols, err := parseWatchlistYAML(data)
			if err != nil {
				return fmt.Errorf("parse yaml %s: %w", filename, err)
			}

			cols := computeColumns(items, explicitCols)
			fetcher := NewQuoteFetcher(5 * time.Second)
			renderTable(os.Stdout, items, cols, fetcher)
			return nil
		},
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
