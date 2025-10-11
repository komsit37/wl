package columns

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/komsit37/wl/pkg/wl/enrich"
	"github.com/komsit37/wl/pkg/wl/types"
)

// Services provides access to external services for resolvers.
type Services struct {
	Quotes enrich.QuoteService
}

// Resolver converts an item into a string value for a given column.
type Resolver func(ctx context.Context, it types.Item, s Services) (string, error)

// Registry maps column keys to resolvers.
var Registry = map[string]Resolver{}

func init() {
	// sym
	Registry["sym"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		return it.Sym, nil
	}
	// name: prefer YAML item name; fallback to quote name
	Registry["name"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		if it.Name != "" {
			return it.Name, nil
		}
		q, _, err := s.Quotes.Get(ctx, it.Sym, enrich.NeedPrice|enrich.NeedChgPct)
		if err != nil {
			return "", nil
		}
		return q.Name, nil
	}
	// price: formatted with yf fmt
	Registry["price"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		q, _, err := s.Quotes.Get(ctx, it.Sym, enrich.NeedPrice)
		if err != nil {
			return "", nil
		}
		return q.Price, nil
	}
	// chg%
	Registry["chg%"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		q, _, err := s.Quotes.Get(ctx, it.Sym, enrich.NeedChgPct)
		if err != nil {
			return "", nil
		}
		return q.ChgFmt, nil
	}
	// sector
	Registry["sector"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		return f.Sector, nil
	}
	// industry
	Registry["industry"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		return f.Industry, nil
	}
	// employees (number with comma separators)
	Registry["employees"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		if f.Employees == 0 {
			return "", nil
		}
		return formatIntComma(f.Employees), nil
	}
	// website (full URL)
	Registry["website"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		return f.Website, nil
	}
	// ir (full URL)
	Registry["ir"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		return f.IR, nil
	}
	// officers_count (number with comma separators)
	Registry["officers_count"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		if f.OfficersCount == 0 {
			return "", nil
		}
		return formatIntComma(f.OfficersCount), nil
	}
	// avg_officer_age (number; typically small; supports thousands just in case)
	Registry["avg_officer_age"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		if f.AvgOfficerAge == nil {
			return "", nil
		}
		// format with one decimal; include comma separators on integer part
		return formatFloatComma(*f.AvgOfficerAge, 1), nil
	}
	// business_summary
	Registry["business_summary"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		return f.BusinessSummary, nil
	}
	// hq: City, Country · Phone · Host(ir|website)
	Registry["hq"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		parts := make([]string, 0, 3)
		loc := strings.TrimSpace(strings.Trim(strings.Join(filterNonEmpty([]string{f.City, f.Country}, ", "), ", "), " "))
		if loc != "" {
			parts = append(parts, loc)
		}
		if f.Phone != "" {
			parts = append(parts, f.Phone)
		}
		// choose IR host if present, else website host
		host := hostOnly(firstNonEmpty(f.IR, f.Website))
		if host != "" {
			parts = append(parts, host)
		}
		return strings.Join(parts, " · "), nil
	}
	// ceo: Name — Title (Age)
	Registry["ceo"] = func(ctx context.Context, it types.Item, s Services) (string, error) {
		_, f, _ := s.Quotes.Get(ctx, it.Sym, enrich.NeedAssetProfile)
		if f.CEOName == "" && f.CEOTitle == "" {
			return "", nil
		}
		base := strings.TrimSpace(strings.Join(filterNonEmpty([]string{f.CEOName}, " "), " "))
		// use em dash style separator
		if f.CEOTitle != "" {
			base = strings.TrimSpace(base + " — " + f.CEOTitle)
		}
		if f.CEOAge != nil {
			base = base + fmt.Sprintf(" (%d)", *f.CEOAge)
		}
		return base, nil
	}
}

// Compute determines final column order from explicit list or inferred.
// Ordering rules: sym -> name -> price -> chg% when sym exists, else
// discover across items, sym first then sorted remainder.
func Compute(explicit []string, items []types.Item) []string {
	// If columns are explicitly provided (via CLI or YAML), honor them exactly.
	if len(explicit) > 0 {
		// Preserve order and do not auto-append defaults.
		// Optionally dedupe while keeping first occurrence.
		seen := map[string]struct{}{}
		out := make([]string, 0, len(explicit))
		for _, k := range explicit {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
		return out
	}

	// Otherwise infer from items with default ordering rules.
	keys := make([]string, 0, 8)
	set := map[string]struct{}{}
	for _, it := range items {
		for k := range it.Fields {
			set[k] = struct{}{}
		}
		if it.Sym != "" {
			set["sym"] = struct{}{}
		}
		if it.Name != "" {
			set["name"] = struct{}{}
		}
	}
	if _, ok := set["sym"]; ok {
		keys = append(keys, "sym")
		delete(set, "sym")
	}
	var rest []string
	for k := range set {
		rest = append(rest, k)
	}
	sort.Strings(rest)
	keys = append(keys, rest...)

	// Ensure name/price/chg% after sym when inferred
	symIdx := -1
	for i, k := range keys {
		if k == "sym" {
			symIdx = i
			break
		}
	}
	if symIdx >= 0 {
		// ensure name
		if !contains(keys, "name") {
			keys = insertAfter(keys, symIdx, "name")
		}
		// ensure price after name (or sym if name absent earlier)
		insertAfterIdx := symIdx
		for i, k := range keys {
			if k == "name" {
				insertAfterIdx = i
				break
			}
		}
		if !contains(keys, "price") {
			keys = insertAfter(keys, insertAfterIdx, "price")
		}
		// ensure chg% after price
		priceIdx := indexOf(keys, "price")
		if priceIdx >= 0 && !contains(keys, "chg%") {
			keys = insertAfter(keys, priceIdx, "chg%")
		}
	}
	return keys
}

func contains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func indexOf(s []string, v string) int {
	for i, e := range s {
		if e == v {
			return i
		}
	}
	return -1
}

func insertAfter(s []string, idx int, v string) []string {
	if idx < 0 || idx >= len(s) {
		return append(s, v)
	}
	s = append(s, "")
	copy(s[idx+2:], s[idx+1:])
	s[idx+1] = v
	return s
}

// NeedForColumns computes a NeedMask for the given columns.
func NeedForColumns(cols []string) enrich.NeedMask {
	var mask enrich.NeedMask
	for _, c := range cols {
		switch c {
		case "price":
			mask |= enrich.NeedPrice
		case "chg%":
			mask |= enrich.NeedChgPct
		case "exchange":
			mask |= enrich.NeedExchange
		case "industry":
			mask |= enrich.NeedAssetProfile
		case "pe":
			mask |= enrich.NeedPE
		case "roe%":
			mask |= enrich.NeedROE
		// AssetProfile-backed columns
		case "sector", "employees", "website", "ir", "officers_count", "avg_officer_age", "business_summary", "hq", "ceo":
			mask |= enrich.NeedAssetProfile
		}
	}
	return mask
}

// RenderValue calls the resolver for the given column.
func RenderValue(ctx context.Context, col string, it types.Item, s Services) (string, error) {
	if r, ok := Registry[col]; ok {
		return r(ctx, it, s)
	}
	// fallback to raw field string
	if v, ok := it.Fields[col]; ok && v != nil {
		return fmt.Sprint(v), nil
	}
	return "", nil
}

// filterNonEmpty joins non-empty trimmed strings using sep; returns slice of non-empty strings.
func filterNonEmpty(parts []string, sep string) []string {
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
	// Try to parse; if missing scheme, add http:// for parsing
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		if strings.Contains(u, "/") {
			// give up, return raw
			return strings.TrimSpace(u)
		}
		return strings.TrimPrefix(strings.TrimPrefix(u, "https://"), "http://")
	}
	h := parsed.Host
	h = strings.TrimPrefix(h, "www.")
	return h
}

// formatIntComma formats an integer with comma thousand separators.
func formatIntComma(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	rem := len(s) % 3
	if rem == 0 {
		rem = 3
	}
	out = append(out, s[:rem]...)
	for i := rem; i < len(s); i += 3 {
		out = append(out, ',')
		out = append(out, s[i:i+3]...)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// formatFloatComma formats a float with a fixed number of decimals and comma separators.
func formatFloatComma(v float64, decimals int) string {
	// Build format string like %.1f
	fmtSpec := fmt.Sprintf("%%.%df", decimals)
	s := fmt.Sprintf(fmtSpec, v)
	// split into integer and fraction
	dot := strings.IndexByte(s, '.')
	if dot == -1 {
		// no fraction
		// try parsing int safely
		return s
	}
	intPart := s[:dot]
	fracPart := s[dot:]
	// handle sign
	sign := ""
	if strings.HasPrefix(intPart, "-") || strings.HasPrefix(intPart, "+") {
		sign = intPart[:1]
		intPart = intPart[1:]
	}
	// comma-format intPart
	n := len(intPart)
	if n <= 3 {
		return sign + intPart + fracPart
	}
	out := make([]byte, 0, len(intPart)+len(intPart)/3)
	rem := n % 3
	if rem == 0 {
		rem = 3
	}
	out = append(out, intPart[:rem]...)
	for i := rem; i < n; i += 3 {
		out = append(out, ',')
		out = append(out, intPart[i:i+3]...)
	}
	return sign + string(out) + fracPart
}
