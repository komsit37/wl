package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/jedib0t/go-pretty/v6/list"

	yfgo "github.com/komsit37/yf-go"

	"github.com/komsit37/wl/pkg/wl/columns"
	"github.com/komsit37/wl/pkg/wl/filter"
	"github.com/komsit37/wl/pkg/wl/pipeline"
	"github.com/komsit37/wl/pkg/wl/render"
	"github.com/komsit37/wl/pkg/wl/source"
)

// resolvePath expands a path that may be:
// - starting with '~' (expanded to the user's home), or
// - relative (resolved against baseDir), or
// - absolute (returned as-is after '~' expansion).
func resolvePath(p string, baseDir string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	if filepath.IsAbs(p) {
		return p
	}
	if baseDir == "" {
		return p
	}
	return filepath.Join(baseDir, p)
}

func main() {
	var (
		flagSource       string
		flagDBDSN        string
		flagOutput       string
		flagNoColor      bool
		flagPretty       bool
		flagCols         string
		flagColSet       string
		flagConfigPath   string
		flagFilter       string
		flagList         bool
		flagListColumns  bool
		flagListColSets  bool
		flagMaxColWidth  int
		flagSortBy       string
		flagSortDesc     bool
		flagCacheDisable bool
		flagCacheTTL     time.Duration
		flagCacheDir     string
	)

	// AppConfig represents configuration loaded from Viper.
	type AppConfig struct {
		Columns    []string            `mapstructure:"columns"`
		ColSet     []string            `mapstructure:"col_set"`
		ColumnSets map[string][]string `mapstructure:"col_sets"`
		// DefaultWatchlist sets the default watchlist path when no CLI path arg is provided.
		// Can be absolute or relative (relative resolves against wlHome).
		DefaultWatchlist string `mapstructure:"default_watchlist"`
		Cache            struct {
			Disabled bool   `mapstructure:"disabled"`
			Dir      string `mapstructure:"dir"`
			TTL      string `mapstructure:"ttl"`
		} `mapstructure:"cache"`
	}

	rootCmd := &cobra.Command{
		Use:   "wl [file|dir]",
		Short: "Render a watchlist",
		Args: func(cmd *cobra.Command, args []string) error {
			// Allow running with no args when listing columns
			listCols, _ := cmd.Flags().GetBool("list-cols")
			listColSets, _ := cmd.Flags().GetBool("list-col-sets")
			if listCols || listColSets {
				return nil
			}
			// Allow 0 or 1 arg; 0 means default watchlist dir under WL_HOME or ~/.wl
			if len(args) > 1 {
				return errors.New("accepts at most 1 path argument (YAML file or directory)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			// Resolve home directory for wl:
			// 1) --config points to a file; its directory becomes WL home if not default
			// 2) WL_HOME or Wl_HOME env var points to base directory
			// 3) default: ~/.wl
			wlHome := os.Getenv("WL_HOME")
			if wlHome == "" {
				wlHome = os.Getenv("Wl_HOME")
			}
			if wlHome == "" {
				userHome, _ := os.UserHomeDir()
				wlHome = filepath.Join(userHome, ".wl")
			}

			// Configure Viper
			vp := viper.New()
			vp.SetConfigType("yaml")
			// If --config specified, use it; otherwise use wlHome/config.yaml
			cfgPath := flagConfigPath
			if strings.TrimSpace(cfgPath) == "" {
				cfgPath = filepath.Join(wlHome, "config.yaml")
			}
			vp.SetConfigFile(cfgPath)
			// Read config only if the file exists; otherwise silently ignore
			if st, err := os.Stat(cfgPath); err == nil && !st.IsDir() {
				if err := vp.ReadInConfig(); err != nil {
					return fmt.Errorf("load config: %w", err)
				}
			}
			// Back-compat alias: allow "col-sets" and "col_set" keys
			// to be recognized alongside "col_sets" / "col_set".
			// We’ll map "col-sets" to ColumnSets if present.
			var cfg AppConfig
			if err := vp.Unmarshal(&cfg); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}
			if cfg.ColumnSets == nil {
				var m map[string][]string
				if err := vp.UnmarshalKey("col-sets", &m); err == nil && len(m) > 0 {
					cfg.ColumnSets = m
				}
			}
			if len(cfg.ColSet) == 0 {
				var sets []string
				if err := vp.UnmarshalKey("col-set", &sets); err == nil && len(sets) > 0 {
					cfg.ColSet = sets
				}
			}
			// Back-compat alias for default watchlist key
			if strings.TrimSpace(cfg.DefaultWatchlist) == "" {
				var s string
				if err := vp.UnmarshalKey("default-watchlist", &s); err == nil && strings.TrimSpace(s) != "" {
					cfg.DefaultWatchlist = s
				}
			}
			// Merge custom column sets from config into built-ins (override on collision)
			if len(cfg.ColumnSets) > 0 {
				for k, v := range cfg.ColumnSets {
					if v == nil {
						continue
					}
					columns.Sets[k] = append([]string(nil), v...)
				}
			}
			// List available columns grouped by YF module (from registry)
			if flagListColumns {
				groups := columns.AvailableByModule()
				// Stable module order preference
				order := []string{"price", "assetProfile", "financialData", "summaryDetail", "base"}
				// Accent group name unless --no-color
				grpStart, grpEnd := "", ""
				if !flagNoColor {
					grpStart, grpEnd = "\x1b[36m", "\x1b[0m" // cyan
				}
				seen := map[string]bool{}
				for _, name := range order {
					if cols, ok := groups[name]; ok && len(cols) > 0 {
						fmt.Fprintf(os.Stdout, "%s%s%s: %s\n", grpStart, name, grpEnd, strings.Join(cols, ","))
						seen[name] = true
					}
				}
				// Print any remaining groups not in preferred order
				keys := make([]string, 0, len(groups))
				for k := range groups {
					if !seen[k] {
						keys = append(keys, k)
					}
				}
				sort.Strings(keys)
				for _, name := range keys {
					cols := groups[name]
					if len(cols) == 0 {
						continue
					}
					fmt.Fprintf(os.Stdout, "%s%s%s: %s\n", grpStart, name, grpEnd, strings.Join(cols, ","))
				}
				return nil
			}

			// Cache configuration merges config defaults with CLI overrides.
			cacheDisabled := cfg.Cache.Disabled
			if cmd.Flags().Changed("cache-disable") {
				cacheDisabled = flagCacheDisable
			}

			var (
				cacheTTL     time.Duration
				haveCacheTTL bool
			)
			if ttlStr := strings.TrimSpace(cfg.Cache.TTL); ttlStr != "" {
				dur, err := time.ParseDuration(ttlStr)
				if err != nil {
					return fmt.Errorf("invalid cache.ttl in config: %w", err)
				}
				if dur <= 0 {
					return fmt.Errorf("cache.ttl must be > 0 (got %s)", ttlStr)
				}
				cacheTTL = dur
				haveCacheTTL = true
			}
			if cmd.Flags().Changed("cache-ttl") {
				if flagCacheTTL <= 0 {
					return errors.New("--cache-ttl must be greater than 0")
				}
				cacheTTL = flagCacheTTL
				haveCacheTTL = true
			}

			cacheDir := ""
			if dir := strings.TrimSpace(cfg.Cache.Dir); dir != "" {
				cacheDir = resolvePath(dir, wlHome)
			}
			if cmd.Flags().Changed("cache-dir") {
				cacheDir = resolvePath(flagCacheDir, wlHome)
			}

			// List column sets (built-in + config) in compact format and exit
			if flagListColSets {
				// Determine module sets vs custom sets (from config.yaml)
				moduleNames := map[string]bool{"price": true, "assetProfile": true, "financialData": true, "summaryDetail": true}
				// Accent set name unless --no-color
				setStart, setEnd := "", ""
				if !flagNoColor {
					setStart, setEnd = "\x1b[36m", "\x1b[0m"
				}

				// Helper to canonicalize and render a set
				renderSet := func(name string, cols []string) {
					can := make([]string, 0, len(cols))
					seen := map[string]struct{}{}
					for _, c := range cols {
						if k, ok := columns.Canonical(c); ok {
							if _, dup := seen[k]; dup {
								continue
							}
							seen[k] = struct{}{}
							can = append(can, k)
						} else {
							if _, dup := seen[c]; dup {
								continue
							}
							seen[c] = struct{}{}
							can = append(can, c)
						}
					}
					fmt.Fprintf(os.Stdout, "%s%s%s: %s\n", setStart, name, setEnd, strings.Join(can, ","))
				}

				// 1) Module sets (price, assetProfile, financialData, summaryDetail) in stable order
				order := []string{"price", "assetProfile", "financialData", "summaryDetail"}
				printedModule := false
				for _, name := range order {
					if cols, ok := columns.Sets[name]; ok && len(cols) > 0 {
						if !printedModule {
							fmt.Fprintf(os.Stdout, "%sMODULE SETS%s\n", setStart, setEnd)
							printedModule = true
						}
						renderSet(name, cols)
					}
				}

				// 2) Custom sets from config.yaml (keys in cfg.ColumnSets that are not module sets)
				// Print in name-sorted order
				customKeys := make([]string, 0, len(cfg.ColumnSets))
				for k := range cfg.ColumnSets {
					if !moduleNames[k] {
						customKeys = append(customKeys, k)
					}
				}
				sort.Strings(customKeys)
				if len(customKeys) > 0 {
					if printedModule {
						fmt.Fprintln(os.Stdout)
					}
					fmt.Fprintf(os.Stdout, "%sCUSTOM SETS%s\n", setStart, setEnd)
				}
				for _, name := range customKeys {
					renderSet(name, columns.Sets[name])
				}
				// Mention special dynamic sets
				if printedModule || len(customKeys) > 0 {
					fmt.Fprintln(os.Stdout)
				}
				fmt.Fprintf(os.Stdout, "%sSPECIAL SETS%s\n", setStart, setEnd)
				fmt.Fprintf(os.Stdout, "%syaml%s: custom fields from YAML (expands per list)\n", setStart, setEnd)
				return nil
			}
			// Source
			var src source.Source
			spec := any(nil)
			switch flagSource {
			case "yaml", "":
				src = source.YAMLSource{}
				// Determine spec path: CLI arg or config default or wlHome/watchlist
				if len(args) == 1 {
					spec = args[0]
				} else {
					def := cfg.DefaultWatchlist
					if strings.TrimSpace(def) == "" {
						def = filepath.Join(wlHome, "watchlist")
					}
					spec = resolvePath(def, wlHome)
				}
			case "db":
				return fmt.Errorf("db source not implemented: dsn=%s", flagDBDSN)
			default:
				return fmt.Errorf("unknown source: %s", flagSource)
			}

			// Renderer
			var rnd render.Renderer
			switch flagOutput {
			case "table", "":
				opts := make([]yfgo.ClientOption, 0, 3)
				if cacheDisabled {
					opts = append(opts, yfgo.WithCacheDisabled())
				} else {
					if cacheDir != "" {
						store, err := yfgo.NewFileCacheStore(cacheDir)
						if err != nil {
							return fmt.Errorf("init cache store (%s): %w", cacheDir, err)
						}
						opts = append(opts, yfgo.WithCacheStore(store))
					}
					if haveCacheTTL {
						opts = append(opts, yfgo.WithDefaultCacheTTL(cacheTTL))
					}
				}
				client := yfgo.NewClient(opts...)
				rnd = render.NewTableRendererWithClient(client)
			case "json":
				rnd = render.NewJSONRenderer()
			default:
				return fmt.Errorf("unknown output: %s", flagOutput)
			}

			// Filter
			f, err := filter.Parse(flagFilter)
			if err != nil {
				return fmt.Errorf("invalid filter: %w", err)
			}

			// List mode: list watchlist names using go-pretty list with hierarchy
			if flagList {
				lists, err := src.Load(cmd.Context(), spec)
				if err != nil {
					return err
				}
				var filt filter.Filter = filter.Always(true)
				if f != nil {
					filt = f
				}
				// Build a tree from filtered watchlist names split by '/'
				type node struct {
					children map[string]*node
					terminal bool
				}
				root := &node{children: map[string]*node{}}
				addPath := func(parts []string) {
					cur := root
					for i, p := range parts {
						if strings.TrimSpace(p) == "" {
							continue
						}
						if cur.children == nil {
							cur.children = map[string]*node{}
						}
						child, ok := cur.children[p]
						if !ok {
							child = &node{children: map[string]*node{}}
							cur.children[p] = child
						}
						cur = child
						if i == len(parts)-1 {
							cur.terminal = true
						}
					}
				}
				for _, wl := range lists {
					if filt.Match(wl.Name) {
						parts := strings.Split(wl.Name, "/")
						addPath(parts)
					}
				}
				// Render the tree using go-pretty with connected rounded style
				lw := list.NewWriter()
				lw.SetStyle(list.StyleConnectedLight)
				lw.SetOutputMirror(os.Stdout)
				var walk func(prefix []string, n *node)
				walk = func(prefix []string, n *node) {
					// sort children for stable output
					keys := make([]string, 0, len(n.children))
					for k := range n.children {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					for i, k := range keys {
						_ = i // order already set by sort
						lw.AppendItem(strings.ToUpper(k))
						child := n.children[k]
						if len(child.children) > 0 {
							lw.Indent()
							walk(append(prefix, k), child)
							lw.UnIndent()
						}
					}
				}
				walk(nil, root)
				_ = list.List{} // ensure package object referenced (example parity)
				_ = lw.Render()
				return nil
			}

			// Columns from config + --col-set and --columns
			var cols []string
			// 1) Column sets: CLI flag takes precedence, else config col_set
			if strings.TrimSpace(flagColSet) != "" {
				parts := strings.Split(flagColSet, ",")
				expanded, err := columns.ExpandSets(parts)
				if err != nil {
					return err
				}
				cols = append(cols, expanded...)
			} else if len(cfg.ColSet) > 0 {
				expanded, err := columns.ExpandSets(cfg.ColSet)
				if err != nil {
					return err
				}
				cols = append(cols, expanded...)
			}
			// 2) Explicit columns: CLI flag takes precedence, else config columns
			if strings.TrimSpace(flagCols) != "" {
				parts := strings.Split(flagCols, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						already := false
						for _, existing := range cols {
							if existing == p {
								already = true
								break
							}
						}
						if !already {
							cols = append(cols, p)
						}
					}
				}
			} else if len(cfg.Columns) > 0 {
				for _, p := range cfg.Columns {
					p = strings.TrimSpace(p)
					if p == "" {
						continue
					}
					already := false
					for _, existing := range cols {
						if existing == p {
							already = true
							break
						}
					}
					if !already {
						cols = append(cols, p)
					}
				}
			}

			// Runner
			run := &pipeline.Runner{
				Source:   src,
				Renderer: rnd,
				Writer:   os.Stdout,
			}
			return run.Execute(cmd.Context(), spec, pipeline.ExecuteOptions{
				Columns:     cols,
				Filter:      f,
				Color:       !flagNoColor,
				PrettyJSON:  flagPretty,
				MaxColWidth: flagMaxColWidth,
				SortBy:      flagSortBy,
				SortDesc:    flagSortDesc,
			})
		},
	}

	rootCmd.Flags().StringVar(&flagSource, "source", "yaml", "data source: yaml|db")
	rootCmd.Flags().StringVar(&flagDBDSN, "db-dsn", "", "database DSN for db source")
	rootCmd.Flags().StringVarP(&flagOutput, "output", "o", "table", "output format: table|json")
	rootCmd.Flags().BoolVar(&flagNoColor, "no-color", false, "disable color output")
	rootCmd.Flags().BoolVarP(&flagPretty, "pretty", "p", false, "pretty-print JSON output")
	rootCmd.Flags().StringVarP(&flagCols, "cols", "c", "", "comma-separated columns to display")
	rootCmd.Flags().StringVarP(&flagColSet, "col-set", "C", "", "comma-separated column sets: price,assetProfile,yaml")
	rootCmd.Flags().StringVar(&flagConfigPath, "config", "", "path to config file (default: $WL_HOME/config.yaml or ~/.wl/config.yaml)")
	rootCmd.Flags().StringVarP(&flagFilter, "filter", "f", "", "filter watchlists by name: substring (ci), name[,name...], glob, or /regex/")
	rootCmd.Flags().BoolVar(&flagList, "list", false, "list watchlist names only")
	rootCmd.Flags().BoolVarP(&flagListColumns, "list-cols", "l", false, "list available column names")
	rootCmd.Flags().BoolVarP(&flagListColSets, "list-col-sets", "L", false, "list column sets in compact form (built-in + config)")
	rootCmd.Flags().IntVar(&flagMaxColWidth, "max-col-width", 40, "max width per column before wrapping (characters)")
	rootCmd.Flags().BoolVar(&flagCacheDisable, "cache-disable", false, "disable Yahoo Finance client caching")
	rootCmd.Flags().DurationVar(&flagCacheTTL, "cache-ttl", 0, "override Yahoo Finance cache TTL (e.g. 2m); 0 keeps library default")
	rootCmd.Flags().StringVar(&flagCacheDir, "cache-dir", "", "use a directory for persistent Yahoo Finance cache entries")
	// Sorting
	rootCmd.Flags().StringVarP(&flagSortBy, "sort", "s", "", "sort rows by column (handles text, numbers, formatted values, and chg%)")
	rootCmd.Flags().BoolVar(&flagSortDesc, "desc", false, "sort in descending order (default asc)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
