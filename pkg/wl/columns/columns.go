package columns

import (
	"context"
	"fmt"
	"sort"

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
}

// Compute determines final column order from explicit list or inferred.
// Ordering rules: sym -> name -> price -> chg% when sym exists, else
// discover across items, sym first then sorted remainder.
func Compute(explicit []string, items []types.Item) []string {
	keys := make([]string, 0, 8)
	if len(explicit) > 0 {
		keys = append(keys, explicit...)
	} else {
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
	}

	// Ensure name/price/chg% after sym
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
			mask |= enrich.NeedIndustry
		case "pe":
			mask |= enrich.NeedPE
		case "roe%":
			mask |= enrich.NeedROE
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
