package filter

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Filter matches a watchlist name.
type Filter interface {
	Match(name string) bool
}

// Parse builds a filter from an expression:
// - Comma-separated exact names: "Core,International"
// - Glob: "Tech*"
// - Regex: "/^US-/"
func Parse(expr string) (Filter, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Always(true), nil
	}
	if strings.HasPrefix(expr, "/") && strings.HasSuffix(expr, "/") && len(expr) > 2 {
		re, err := regexp.Compile(expr[1 : len(expr)-1])
		if err != nil {
			return nil, err
		}
		return Regex{re: re}, nil
	}
	if strings.Contains(expr, ",") {
		parts := strings.Split(expr, ",")
		set := map[string]struct{}{}
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			set[p] = struct{}{}
		}
		return ExactSet{set: set}, nil
	}
	if strings.ContainsAny(expr, "*?") {
		return Glob{pattern: expr}, nil
	}
	// Default: case-insensitive substring match (equivalent to *expr*).
	return SubstrCI{needle: expr}, nil
}

// Implementations

type Always bool

func (a Always) Match(string) bool { return bool(a) }

type Exact struct{ value string }

func (e Exact) Match(name string) bool { return name == e.value }

type ExactSet struct{ set map[string]struct{} }

func (e ExactSet) Match(name string) bool {
	_, ok := e.set[name]
	return ok
}

type Glob struct{ pattern string }

func (g Glob) Match(name string) bool {
	ok, _ := filepath.Match(g.pattern, name)
	return ok
}

type Regex struct{ re *regexp.Regexp }

func (r Regex) Match(name string) bool { return r.re.MatchString(name) }

// String provides a human-readable representation useful for logs/errors.
func (g Glob) String() string  { return fmt.Sprintf("glob:%s", g.pattern) }
func (e Exact) String() string { return fmt.Sprintf("exact:%s", e.value) }

// SubstrCI matches if name contains needle, case-insensitively.
type SubstrCI struct{ needle string }

func (s SubstrCI) Match(name string) bool {
	if s.needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(s.needle))
}

func (s SubstrCI) String() string { return fmt.Sprintf("substr-ci:%s", s.needle) }
