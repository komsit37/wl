% wl Refactor Plan

This plan refactors `cmd/wl/main.go` into a thin CLI over a reusable library while enabling:
- Alternate data sources (e.g., DB)
- Alternate renderers (JSON)
- Library publishing (`pkg/wl`)
- Price/fundamentals cache
- Optional columns (exchange, industry, P/E, ROE%)
- Multiple watchlists
- Filtering by watchlist name via `--filter`

## Goals
- Decouple source loading, enrichment, and rendering.
- Keep CLI orchestration-only; move logic to `pkg/wl` with stable, public APIs.
- Make columns extensible without changing the CLI.
- Add multi-watchlist support and JSON output.
- Implement an in-memory cache with TTL (pluggable later).

## Scope Clarification
- `--filter` applies to watchlist names only (not items).

## Package Layout (Library: `pkg/wl`)

### `pkg/wl/types`
- `type Watchlist struct { Name string; Columns []string; Items []Item }`
- `type Item struct { Sym string; Name string; Fields map[string]any }`
- `type Quote struct { Price string; ChgFmt string; ChgRaw float64; Name string }`

### `pkg/wl/source`
- `type Source interface { Load(ctx context.Context, spec any) ([]types.Watchlist, error) }`
- `YAMLSource` (migrate current YAML parsing; support nested groups => multiple watchlists)
- `DBSource` (stubbed; accepts DSN/driver; implement later)

### `pkg/wl/enrich`
- `type NeedMask uint64` // bitmask for Price, ChgPct, Exchange, Industry, PE, ROE
- `type QuoteService interface { Get(ctx context.Context, sym string, need NeedMask) (types.Quote, Fundamentals, error) }`
- `YFService` uses `yf-go`, requesting only necessary modules based on `NeedMask`.
- `Cache` decorator with TTL + size (LRU). Keyed by `sym|needMask`.

### `pkg/wl/columns`
- Column registry: `map[string]Resolver`
- `type Resolver func(ctx context.Context, it types.Item, s Services) (string, error)`
- Built-ins: `sym`, `name`, `price`, `chg%`, `exchange`, `industry`, `pe`, `roe%`.
- `Compute(explicit []string, items []types.Item) []string` preserving current ordering rules (sym → name → price → chg%).

### `pkg/wl/render`
- `type Renderer interface { Render(w io.Writer, lists []types.Watchlist, opts RenderOptions) error }`
- `type RenderOptions struct { Columns []string; Color bool; PrettyJSON bool }`
- `TableRenderer` (go-pretty) with section headers per watchlist.
- `JSONRenderer` (machine-readable; optional pretty output, raw values only — no formatted strings).

### `pkg/wl/filter`
- `type Filter interface { Match(name string) bool }`
- `Parse(expr string) (Filter, error)` supporting:
  - Comma-separated exact names: `Core,International`
  - Glob: `Tech*`
  - Regex: `/^US-/`

### `pkg/wl/pipeline`
- `type Runner struct { Source source.Source; Quotes enrich.QuoteService; Renderer render.Renderer }`
- `type ExecuteOptions struct { Columns []string; Filter filter.Filter; PriceCacheTTL time.Duration; Concurrency int }`
- `func (r *Runner) Execute(ctx context.Context, spec any, opts ExecuteOptions) error`

## CLI (`cmd/wl/main.go`)
- Keep minimal: parse flags, compose library components, call `Runner.Execute`.
- Flags:
  - `--source yaml|db` (default: yaml)
  - `--db-dsn <dsn>` (for future DBSource)
  - `--output table|json` (default: table)
  - `--no-color`, `--pretty-json`
  - `--columns "sym,name,price,chg%,exchange,industry,pe,roe%"`
  - `--filter "<name[,name...]|glob|/regex/>"` // filters watchlists by name only
  - `--price-cache-ttl 5s`, `--price-cache-size 1000`
  - `--concurrency 5` (default)

## Data Flow
1. Source loads `[]Watchlist` (YAML/DB).
2. Apply name filter to choose watchlists.
3. Determine columns per list (explicit overrides; else discovered + rules).
4. Resolve column values using registry; quote/fundamentals via `QuoteService` with cache.
5. Render via selected renderer (table/json).

## Caching
- In-memory LRU with TTL; config via flags.
- Cache fronts `QuoteService.Get`; key includes `needMask` to avoid mixing scopes.
- Swappable in future (e.g., Redis) by wrapping the interface.

## Rendering
- Table: mirror current style (no borders, compact), 10-char name trim, colorized price/change.
- JSON: `[{ name, columns, items: [{ sym, name, fields: { col: value } }] }]` with raw values only (e.g., `price` and `pe` as numbers, `chg%` as numeric percent; no color/formatting).

## Sources
- YAML: move existing parser; now returns multiple named watchlists.
- DB: introduce interface and flags; return "not implemented" error until schema is defined.

## Incremental Milestones
1. Extract YAML parsing to `pkg/wl/source/yaml.go` returning `[]Watchlist` with names.
2. Extract table rendering to `pkg/wl/render/table.go`; keep current visuals.
3. Introduce `pkg/wl/types`; wire CLI to use source → columns → table renderer (parity with today).
4. Add `pkg/wl/filter` (name-based) and CLI `--filter`.
5. Introduce `enrich.QuoteService` with TTL cache; port current price/chg% logic.
6. Add `columns` registry; implement resolvers for `sym,name,price,chg%`.
7. Add resolvers for `exchange,industry,pe,roe%` via `YFService` modules.
8. Add `JSONRenderer` and `--output json` + `--pretty-json`.
9. Add `DBSource` skeleton and CLI flags (returns not implemented).

## Testing
- YAML parsing: columns + nested groups → multiple lists.
- Column computation: explicit vs inferred; ordering rules.
- Filter: exact, glob, regex name matching.
- Quote caching: TTL behavior, mask separation, size limit.
- Renderers: table snapshot (golden), JSON shape w/ and w/o pretty.

## Backward Compatibility
- Default behavior remains: `wl <file.yaml>` renders a single table.
- Legacy YAML formats can be adapted or rejected with clear errors (prefer adapter if low effort).

## Open Questions
None — resolved below.

## Decisions
- Naming: Use `name` when provided; otherwise derive hierarchical path from nested group names (joined by `/`).
- JSON: Emit raw values only (no formatted strings). Numeric columns remain numeric (e.g., `price`, `pe`, `roe%`, `chg%`).
- Concurrency: Limit to 5 concurrent requests by default (configurable via `--concurrency`).

## Out of Scope (for this phase)
- Implementing actual DB schema/queries.
- Distributed or persistent cache backends.
- Item-level filtering or sorting.
