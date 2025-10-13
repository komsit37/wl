package render

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	yfgo "github.com/komsit37/yf-go"

	"github.com/komsit37/wl/pkg/wl/columns"
	"github.com/komsit37/wl/pkg/wl/types"
)

type TableRenderer struct{ Client *yfgo.Client }

func NewTableRenderer() *TableRenderer { return &TableRenderer{Client: yfgo.NewClient()} }

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

		// Column configs: wrap text to MaxColWidth (default 40), no truncation
		maxWidth := opts.MaxColWidth
		if maxWidth <= 0 {
			maxWidth = 40
		}
		cfgs := make([]table.ColumnConfig, 0, len(cols))
		for i, c := range cols {
			cfg := table.ColumnConfig{Number: i + 1, WidthMax: maxWidth}
			switch c {
			case "employees", "officers_count", "avg_officer_age":
				cfg.Align = text.AlignRight
				cfg.AlignHeader = text.AlignRight
			}
			cfgs = append(cfgs, cfg)
		}
		if len(cfgs) > 0 {
			tw.SetColumnConfigs(cfgs)
		}

		// Rows
		for _, it := range list.Items {
			// Compute required modules and fetch once per symbol
			mods := columns.RequiredModules(cols)
			raw, err := r.Client.QuoteSummary(context.Background(), it.Sym, mods)
			if err != nil {
				raw = nil
			}
			m := columns.RawToMap(raw)

			row := make(table.Row, len(cols))
			for i, c := range cols {
				key := c
				if k, ok := columns.Canonical(c); ok {
					key = k
				}
				row[i] = renderFromRaw(key, it, m)
				// Colorize for price and chg%
				if opts.Color && (key == "price" || key == "chg%") {
					// read raw change percent
					if v, ok := columns.Extract(m, "price.regularMarketChangePercent.raw"); ok {
						if strings.HasPrefix(v, "-") { // negative
							row[i] = text.Colors{text.FgRed}.Sprintf("%v", row[i])
						} else if v != "" && v != "0" && v != "0.0" && v != "0.00" { // positive
							row[i] = text.Colors{text.FgGreen}.Sprintf("%v", row[i])
						}
					}
				}
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

// renderFromRaw extracts a value for a canonical key from raw map, with fallbacks.
func renderFromRaw(key string, it types.Item, m map[string]any) string {
	switch key {
	case "sym":
		return it.Sym
	case "name":
		if it.Name != "" {
			return it.Name
		}
		if v, ok := columns.Extract(m, "price.shortName|price.longName"); ok {
			return v
		}
		return ""
	case "avg_officer_age":
		return avgOfficerAge(m)
	case "hq":
		return hqFromRaw(m)
	case "ceo":
		return ceoFromRaw(m)
	default:
		if def, ok := columns.GetDef(key); ok && strings.TrimSpace(def.Path) != "" {
			if v, ok := columns.Extract(m, def.Path); ok {
				return v
			}
		}
		return ""
	}
}

func avgOfficerAge(m map[string]any) string {
	v, _ := columns.Extract(m, "assetProfile.companyOfficers")
	// direct extraction returns JSON; parse array
	var arr []map[string]any
	if b := []byte(v); len(b) > 0 && b[0] == '[' {
		_ = json.Unmarshal(b, &arr)
	}
	if len(arr) == 0 {
		return ""
	}
	var sum float64
	var cnt int
	for _, o := range arr {
		if a, ok := o["age"]; ok {
			switch t := a.(type) {
			case float64:
				sum += t
				cnt++
			case json.Number:
				if f, err := t.Float64(); err == nil {
					sum += f
					cnt++
				}
			}
		}
	}
	if cnt == 0 {
		return ""
	}
	avg := sum / float64(cnt)
	return columns.FormatFloat(avg, 1)
}

func hqFromRaw(m map[string]any) string {
	city, _ := columns.Extract(m, "assetProfile.city")
	country, _ := columns.Extract(m, "assetProfile.country")
	phone, _ := columns.Extract(m, "assetProfile.phone")
	ir, _ := columns.Extract(m, "assetProfile.irWebsite")
	web, _ := columns.Extract(m, "assetProfile.website")
	// Join city and country with proper separators and trim spaces
	loc := strings.TrimSpace(strings.Join(filterNonEmpty([]string{city, country}), ", "))
	parts := make([]string, 0, 3)
	if loc != "" {
		parts = append(parts, loc)
	}
	if phone != "" {
		parts = append(parts, phone)
	}
	host := hostOnly(firstNonEmpty(ir, web))
	if host != "" {
		parts = append(parts, host)
	}
	return strings.Join(parts, " · ")
}

func ceoFromRaw(m map[string]any) string {
	// parse officers and choose best by title
	v, _ := columns.Extract(m, "assetProfile.companyOfficers")
	var arr []map[string]any
	if b := []byte(v); len(b) > 0 && b[0] == '[' {
		_ = json.Unmarshal(b, &arr)
	}
	if len(arr) == 0 {
		return ""
	}
	bestIdx := -1
	for i, o := range arr {
		title, _ := o["title"].(string)
		lt := strings.ToLower(title)
		if strings.Contains(lt, "ceo") || strings.Contains(lt, "president") || strings.Contains(lt, "representative director") {
			bestIdx = i
			break
		}
	}
	if bestIdx == -1 {
		bestIdx = 0
	}
	o := arr[bestIdx]
	name, _ := o["name"].(string)
	title, _ := o["title"].(string)
	var ageStr string
	if age, ok := o["age"]; ok {
		ageStr = fmt.Sprint(age)
		if ageStr != "" {
			ageStr = " (" + ageStr + ")"
		}
	}
	base := strings.TrimSpace(strings.Join(filterNonEmpty([]string{name}), " "))
	if title != "" {
		base = strings.TrimSpace(base + " — " + title)
	}
	return base + ageStr
}

// Utilities copied locally to avoid export churn
func filterNonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func hostOnly(u string) string {
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		if strings.Contains(u, "/") {
			return strings.TrimSpace(u)
		}
		return strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	}
	h := parsed.Host
	h = strings.TrimPrefix(h, "www.")
	return h
}
