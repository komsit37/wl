package columns

import "strings"

// Sets defines named column groups that expand into lists of columns.
// It is initialized from ColumnDef so that each Yahoo module has a set.
// User config can merge/override in cmd/wl/main.go.
var Sets = map[string][]string{}

// BuildDefaultSetsFromDefs populates Sets with one set per module from ColumnDef.
// Excludes the "base" group (non-Yahoo backed fields like sym/name unless mapped).
func BuildDefaultSetsFromDefs() {
	groups := AvailableByModule()
	out := make(map[string][]string, len(groups))
	for name, cols := range groups {
		if name == "base" { // skip base group
			continue
		}
		out[name] = append([]string(nil), cols...)
	}
	Sets = out
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
		// Special dynamic set: "yaml" expands later based on items.
		// We pass the token through so downstream can expand per-list.
		if name == "yaml" {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				out = append(out, name)
			}
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
