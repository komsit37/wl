package columns

import "strings"

// Sets defines named column groups that expand into lists of columns.
// - "price": price-related columns
// - "assetProfile": columns backed by Yahoo Finance assetProfile module
var Sets = map[string][]string{
    // Price-related columns
    "price": {"price", "chg%"},
    // Asset profile-backed columns
    "assetProfile": {
        "sector",
        "industry",
        "employees",
        "website",
        "ir",
        "officers_count",
        "avg_officer_age",
        "business_summary",
        "hq",
        "ceo",
    },
}

// ExpandSets returns the union of columns for the given set names.
// It preserves the order of the sets and the order of columns within each set,
// and de-duplicates columns while keeping the first occurrence.
func ExpandSets(setNames []string) ([]string, error) {
    out := make([]string, 0, 16)
    seen := map[string]struct{}{}
    for _, name := range setNames {
        name = strings.TrimSpace(name)
        if name == "" {
            continue
        }
        cols, ok := Sets[name]
        if !ok {
            // Unknown set is an error; surface clear message to caller.
            return nil, &UnknownSetError{Name: name, Available: availableSets()}
        }
        for _, c := range cols {
            if _, ok := seen[c]; ok {
                continue
            }
            seen[c] = struct{}{}
            out = append(out, c)
        }
    }
    return out, nil
}

// UnknownSetError reports an unknown column set name.
type UnknownSetError struct {
    Name      string
    Available []string
}

func (e *UnknownSetError) Error() string {
    return "unknown column set: " + e.Name + "; available: " + strings.Join(e.Available, ", ")
}

func availableSets() []string {
    keys := make([]string, 0, len(Sets))
    for k := range Sets {
        keys = append(keys, k)
    }
    // keep natural map order sufficient; no sorting dependency here
    return keys
}

