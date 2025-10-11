package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/komsit37/wl/pkg/wl/types"
)

// YAMLSource loads watchlists from a YAML file.
type YAMLSource struct{}

// Load expects spec to be a string filepath.
func (YAMLSource) Load(ctx context.Context, spec any) ([]types.Watchlist, error) { //nolint:revive // ctx reserved for future use
	path, ok := spec.(string)
	if !ok {
		return nil, fmt.Errorf("yaml source expects filepath string spec")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		// Recursively load all YAML files in the directory and combine.
		var files []string
		err := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext == ".yaml" || ext == ".yml" {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)

		var all []types.Watchlist
		for _, full := range files {
			f, err := os.Open(full)
			if err != nil {
				return nil, err
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil {
				return nil, err
			}
			lists, err := parseYAML(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", full, err)
			}
			// Compute prefix from relative path (without extension), using forward slashes.
			rel, err := filepath.Rel(path, full)
			if err != nil {
				rel = filepath.Base(full)
			}
			ext := filepath.Ext(rel)
			prefix := strings.TrimSuffix(rel, ext)
			prefix = filepath.ToSlash(prefix)
			for i := range lists {
				if strings.TrimSpace(lists[i].Name) == "" {
					lists[i].Name = prefix
				} else if prefix != "" {
					lists[i].Name = prefix + "/" + lists[i].Name
				}
			}
			all = append(all, lists...)
		}
		return all, nil
	}

	// Single file
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	lists, err := parseYAML(data)
	if err != nil {
		return nil, err
	}
	// If a list has no name, use the file name as a fallback.
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	for i := range lists {
		if strings.TrimSpace(lists[i].Name) == "" {
			lists[i].Name = base
		}
	}
	return lists, nil
}

// parseYAML parses the repo's YAML format into multiple watchlists.
func parseYAML(data []byte) ([]types.Watchlist, error) {
	var root any
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	// Normalize maps with non-string keys to map[string]any
	var norm func(v any) any
	norm = func(v any) any {
		switch m := v.(type) {
		case map[any]any:
			mm := make(map[string]any, len(m))
			for k, val := range m {
				mm[fmt.Sprint(k)] = norm(val)
			}
			return mm
		case []any:
			out := make([]any, 0, len(m))
			for _, e := range m {
				out = append(out, norm(e))
			}
			return out
		default:
			return v
		}
	}
	root = norm(root)

	m, ok := root.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid yaml: expected map with 'watchlist'")
	}

	var explicitCols []string
	if v, ok := m["columns"]; ok && v != nil {
		explicitCols = toStringSlice(v)
	}

	wlNode, ok := m["watchlist"]
	if !ok || wlNode == nil {
		return nil, fmt.Errorf("invalid yaml: missing 'watchlist'")
	}

	// Traverse to produce lists.
	var lists []types.Watchlist
	// Accumulate path of group names.
	var walk func(node any, path []string)
	walk = func(node any, path []string) {
		switch n := node.(type) {
		case []any:
			// Items or groups in a list; but only produce a list when encountering
			// a named group or a plain list at root with leaf items.
			// Detect if this list contains any leaf items; if so, make a list.
			leafItems := make([]types.Item, 0)
			for _, e := range n {
				if isLeaf(e) {
					it := toItem(e)
					leafItems = append(leafItems, it)
				}
			}
			if len(leafItems) > 0 {
				lists = append(lists, types.Watchlist{
					Name:    deriveName(path),
					Columns: append([]string(nil), explicitCols...),
					Items:   leafItems,
				})
			}
			// Also traverse groups within this list.
			for _, e := range n {
				if g, ok := e.(map[string]any); ok {
					if child, ok := g["watchlist"]; ok {
						var nextPath []string
						if name, ok := g["name"].(string); ok && name != "" {
							nextPath = append(append([]string(nil), path...), name)
						} else {
							nextPath = append([]string(nil), path...)
						}
						walk(child, nextPath)
					}
				}
			}
		case map[string]any:
			if child, ok := n["watchlist"]; ok {
				var nextPath []string
				if name, ok := n["name"].(string); ok && name != "" {
					nextPath = append(append([]string(nil), path...), name)
				} else {
					nextPath = append([]string(nil), path...)
				}
				walk(child, nextPath)
				return
			}
			// Single leaf at map level
			if isLeaf(n) {
				lists = append(lists, types.Watchlist{
					Name:    deriveName(path),
					Columns: append([]string(nil), explicitCols...),
					Items:   []types.Item{toItem(n)},
				})
			}
		}
	}

	walk(wlNode, nil)
	return lists, nil
}

func toStringSlice(v any) []string {
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

func isLeaf(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := m["watchlist"]; ok {
		return false
	}
	// Consider a leaf if it has at least a sym or other fields.
	_, hasSym := m["sym"]
	return hasSym || len(m) > 0
}

func toItem(v any) types.Item {
	m, _ := v.(map[string]any)
	it := types.Item{Fields: map[string]any{}}
	if sym, ok := m["sym"]; ok && sym != nil {
		it.Sym = fmt.Sprint(sym)
		it.Fields["sym"] = it.Sym
	}
	if name, ok := m["name"]; ok && name != nil {
		it.Name = fmt.Sprint(name)
		it.Fields["name"] = it.Name
	}
	for k, val := range m {
		if k == "sym" || k == "name" || k == "watchlist" {
			continue
		}
		it.Fields[k] = val
	}
	return it
}

func deriveName(path []string) string {
	if len(path) == 0 {
		return ""
	}
	return strings.Join(path, "/")
}
