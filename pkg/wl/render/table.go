package render

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
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

		// Pre-fetch and compute sort keys
		type rowData struct {
			it       types.Item
			raw      map[string]any
			dispSort string
			numSort  float64
			hasNum   bool
			missing  bool
		}

		rows := make([]rowData, 0, len(list.Items))
		// Determine modules needed for display columns plus possibly sort column
		neededCols := cols
		if strings.TrimSpace(opts.SortBy) != "" {
			// ensure sort column is included for module calc
			neededCols = append(append([]string(nil), cols...), opts.SortBy)
		}
		mods := columns.RequiredModules(neededCols)
		for _, it := range list.Items {
			raw, err := r.Client.QuoteSummary(context.Background(), it.Sym, mods)
			if err != nil {
				raw = nil
			}
			m := columns.RawToMap(raw)
			rd := rowData{it: it, raw: m}
			if strings.TrimSpace(opts.SortBy) != "" {
				rd.dispSort, rd.numSort, rd.hasNum, rd.missing = computeSortKey(opts.SortBy, it, m)
			}
			rows = append(rows, rd)
		}

		// Sort if requested
		if strings.TrimSpace(opts.SortBy) != "" {
			sort.SliceStable(rows, func(i, j int) bool {
				a, b := rows[i], rows[j]
				// Missing values sort last
				if a.missing && b.missing {
					return false
				}
				if a.missing {
					return false
				}
				if b.missing {
					return true
				}
				// Numeric compare when both numeric
				if a.hasNum && b.hasNum {
					if opts.SortDesc {
						if a.numSort == b.numSort {
							return strings.Compare(a.dispSort, b.dispSort) > 0
						}
						return a.numSort > b.numSort
					}
					if a.numSort == b.numSort {
						return strings.Compare(a.dispSort, b.dispSort) < 0
					}
					return a.numSort < b.numSort
				}
				// Fallback to case-insensitive lexicographic compare
				ad, bd := strings.ToLower(a.dispSort), strings.ToLower(b.dispSort)
				if opts.SortDesc {
					if ad == bd {
						return a.dispSort > b.dispSort
					}
					return ad > bd
				}
				if ad == bd {
					return a.dispSort < b.dispSort
				}
				return ad < bd
			})
		}

		// Render rows
		for _, rdata := range rows {
			it, m := rdata.it, rdata.raw
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
		// 1) Built-in/YF-backed columns via registered path
		if def, ok := columns.GetDef(key); ok && strings.TrimSpace(def.Path) != "" {
			if v, ok := columns.Extract(m, def.Path); ok {
				return v
			}
		}
		// 2) Custom YAML fields: fall back to item fields (case-insensitive)
		if it.Fields != nil {
			if v, ok := it.Fields[key]; ok && v != nil {
				return strings.TrimSpace(fmt.Sprint(v))
			}
			lk := strings.ToLower(key)
			for k, v := range it.Fields {
				if strings.ToLower(k) == lk && v != nil {
					return strings.TrimSpace(fmt.Sprint(v))
				}
			}
		}
		return ""
	}
}

// computeSortKey derives display string and best-effort numeric value for sorting.
// It handles known YF-backed columns (preferring raw values), YAML custom fields,
// formatted strings (currency, K/M/B/T), and percentages like chg%.
func computeSortKey(col string, it types.Item, m map[string]any) (disp string, num float64, hasNum bool, missing bool) {
	key := col
	if k, ok := columns.Canonical(col); ok {
		key = k
	}
	// Display value using render to ensure consistent fallback behavior
	disp = renderFromRaw(key, it, m)
	d := strings.TrimSpace(disp)
	if d == "" {
		return disp, 0, false, true
	}

	// Try to extract numeric raw for known keys
	// Special-case chg%
	if key == "chg%" {
		if v, ok := columns.Extract(m, "price.regularMarketChangePercent.raw"); ok {
			if f, err := parseFloatStrict(v); err == nil {
				return disp, f, true, false
			}
		}
	}
	// If key has a registered def with a .fmt path, try .raw first
	if def, ok := columns.GetDef(key); ok && strings.Contains(def.Path, ".fmt") {
		rawPath := strings.Replace(def.Path, ".fmt", ".raw", 1)
		if v, ok := columns.Extract(m, rawPath); ok {
			if f, err := parseFloatStrict(v); err == nil {
				return disp, f, true, false
			}
		}
	}

	// Fallback: parse formatted text (currency, percent, K/M/B/T)
	if f, ok := parseFormattedNumber(d); ok {
		return disp, f, true, false
	}

	// YAML custom fields may be numeric; try to parse directly from item.Fields case-insensitively
	if it.Fields != nil {
		lkey := strings.ToLower(key)
		for fk, fv := range it.Fields {
			if strings.ToLower(fk) == lkey {
				s := strings.TrimSpace(fmt.Sprint(fv))
				if s != "" {
					if f, err := parseFloatStrict(s); err == nil {
						return disp, f, true, false
					}
					if f, ok := parseFormattedNumber(s); ok {
						return disp, f, true, false
					}
				}
				break
			}
		}
	}

	// Not numeric; use display string for lexicographic sort
	return disp, 0, false, false
}

// parseFloatStrict tries to parse a plain float string.
func parseFloatStrict(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// parseFormattedNumber parses values like "$1,234.56", "1.2B", "-3.4%", "(5.6)", etc.
func parseFormattedNumber(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	t := strings.TrimSpace(s)
	// Handle parentheses for negatives
	neg := false
	if strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		neg = true
		t = strings.TrimSpace(t[1 : len(t)-1])
	}
	// Strip currency symbols and spaces
	// Keep digits, signs, dot, percent, and K/M/B/T suffixes
	cleaned := make([]rune, 0, len(t))
	for _, r := range t {
		if (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '+' || r == '%' || r == 'K' || r == 'M' || r == 'B' || r == 'T' || r == 'k' || r == 'm' || r == 'b' || r == 't' {
			cleaned = append(cleaned, r)
		}
	}
	u := string(cleaned)
	if u == "" {
		return 0, false
	}
	// Percent
	isPct := strings.HasSuffix(u, "%")
	if isPct {
		u = strings.TrimSuffix(u, "%")
	}
	// Suffix multiplier
	mult := 1.0
	if len(u) > 0 {
		last := u[len(u)-1]
		switch last {
		case 'K', 'k':
			mult = 1e3
			u = u[:len(u)-1]
		case 'M', 'm':
			mult = 1e6
			u = u[:len(u)-1]
		case 'B', 'b':
			mult = 1e9
			u = u[:len(u)-1]
		case 'T', 't':
			mult = 1e12
			u = u[:len(u)-1]
		}
	}
	f, err := strconv.ParseFloat(u, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		f = -f
	}
	if isPct {
		// Keep as percentage value (e.g., 5.3% => 5.3)
		// Do not convert to fraction; sorting by displayed percent is expected
	}
	return f * mult, true
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
