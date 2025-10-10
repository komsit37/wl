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
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	yfgo "github.com/komsit37/yf-go"
)

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
			viper.AutomaticEnv()

			filename := args[0]
			f, err := os.Open(filename)
			if err != nil {
				return fmt.Errorf("open %s: %w", filename, err)
			}
			defer f.Close()

			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read %s: %w", filename, err)
			}

			// Support two YAML shapes:
			// 1) Top-level list of items: "- sym: ..."
			// 2) Map with optional columns and items: "columns: [...]; items: [...]"
			var (
				items []map[string]any
				keys  []string
			)

			// Try list form: top-level list of maps
			if err := yaml.Unmarshal(data, &items); err != nil {
				// Try map form: object with columns + items
				var alt struct {
					Columns []string         `yaml:"columns"`
					Items   []map[string]any `yaml:"items"`
				}
				if err2 := yaml.Unmarshal(data, &alt); err2 != nil {
					return fmt.Errorf("parse yaml %s: %w", filename, err)
				}
				items = alt.Items
				// If columns provided, respect order; otherwise compute fallback below.
				if len(alt.Columns) > 0 {
					keys = append(keys, alt.Columns...)
				}
			}

			// If keys not set yet, compute from items with sensible ordering.
			if len(keys) == 0 {
				// Determine all keys across items, put "sym" first if present, then others sorted.
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

			// Ensure a computed "price" column exists right after "sym" if present
			// and not already included. Also ensure a computed "chg%" right after "price".
			// If no "sym" column, leave keys as-is.
			{
				// Find sym index
				symIdx := -1
				for i, k := range keys {
					if k == "sym" {
						symIdx = i
						break
					}
				}
				if symIdx >= 0 {
					// Check if price already present
					foundPrice := false
					for _, k := range keys {
						if k == "price" {
							foundPrice = true
							break
						}
					}
					if !foundPrice {
						// Insert "price" after symIdx
						keys = append(keys, "")
						copy(keys[symIdx+2:], keys[symIdx+1:])
						keys[symIdx+1] = "price"
					}

					// Ensure "chg%" exists right after "price"
					priceIdx := -1
					for i, k := range keys {
						if k == "price" {
							priceIdx = i
							break
						}
					}
					if priceIdx >= 0 {
						foundChg := false
						for _, k := range keys {
							if k == "chg%" {
								foundChg = true
								break
							}
						}
						if !foundChg {
							keys = append(keys, "")
							copy(keys[priceIdx+2:], keys[priceIdx+1:])
							keys[priceIdx+1] = "chg%"
						}
					}
				}
			}

			tw := table.NewWriter()
			tw.SetOutputMirror(os.Stdout)
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

			// Prepare quote fetcher with simple in-memory cache
			type quoteOut struct {
				price string
				chgFmt string
				chgRaw float64
			}
			quoteCache := map[string]quoteOut{}
			client := yfgo.NewClient()
			fetchQuote := func(ctx context.Context, sym string) quoteOut {
				if sym == "" {
					return quoteOut{}
				}
				if v, ok := quoteCache[sym]; ok {
					return v
				}
				ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				defer cancel()
				res, err := client.QuoteSummaryTyped(ctx, sym, []yfgo.QuoteSummaryModule{yfgo.ModulePrice})
				if err != nil || res.Price == nil {
					quoteCache[sym] = quoteOut{}
					return quoteOut{}
				}
				// Prefer formatted value if available, otherwise raw
				p := res.Price.RegularMarketPrice
				var priceStr string
				if p.Fmt != "" {
					priceStr = p.Fmt
				} else if p.Raw != nil {
					priceStr = fmt.Sprintf("%.2f", *p.Raw)
				} else {
					priceStr = ""
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
				qo := quoteOut{price: priceStr, chgFmt: chgFmt, chgRaw: chgRaw}
				quoteCache[sym] = qo
				return qo
			}

			// Rows
			for _, m := range items {
				row := make(table.Row, len(keys))
				var symVal string
				if v, ok := m["sym"]; ok && v != nil {
					symVal = fmt.Sprint(v)
				}
				qo := fetchQuote(context.Background(), symVal)
				for i, k := range keys {
					if k == "price" || k == "chg%" {
						val := ""
						if k == "price" {
							val = qo.price
						} else {
							val = qo.chgFmt
						}
						if qo.chgRaw > 0 {
							val = text.Colors{text.FgGreen}.Sprintf("%s", val)
						} else if qo.chgRaw < 0 {
							val = text.Colors{text.FgRed}.Sprintf("%s", val)
						}
						row[i] = val
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
			return nil
		},
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
