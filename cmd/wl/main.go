package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jedib0t/go-pretty/v6/list"

	"github.com/komsit37/wl/pkg/wl/columns"
	"github.com/komsit37/wl/pkg/wl/enrich"
	"github.com/komsit37/wl/pkg/wl/filter"
	"github.com/komsit37/wl/pkg/wl/pipeline"
	"github.com/komsit37/wl/pkg/wl/render"
	"github.com/komsit37/wl/pkg/wl/source"
)

func main() {
	var (
		flagSource      string
		flagDBDSN       string
		flagOutput      string
		flagNoColor     bool
		flagPrettyJSON  bool
		flagColumns     string
		flagFilter      string
		flagCacheTTL    time.Duration
		flagCacheSize   int
		flagConcurrency int
		flagList        bool
	)

	rootCmd := &cobra.Command{
		Use:   "wl <file|dir>",
		Short: "Render a watchlist",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("requires exactly 1 path argument (YAML file or directory)")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			// Source
			var src source.Source
			spec := any(nil)
			switch flagSource {
			case "yaml", "":
				src = source.YAMLSource{}
				spec = args[0]
			case "db":
				return fmt.Errorf("db source not implemented: dsn=%s", flagDBDSN)
			default:
				return fmt.Errorf("unknown source: %s", flagSource)
			}

			// Quotes service with cache
			qs := enrich.NewYFService(5 * time.Second)
			if flagCacheTTL > 0 && flagCacheSize <= 0 {
				flagCacheSize = 1000
			}

			// Renderer
			var rnd render.Renderer
			services := columns.Services{Quotes: qs}
			switch flagOutput {
			case "table", "":
				rnd = render.NewTableRenderer(services)
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

			// Columns
			var cols []string
			if strings.TrimSpace(flagColumns) != "" {
				parts := strings.Split(flagColumns, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						cols = append(cols, p)
					}
				}
			}

			// Runner
			run := &pipeline.Runner{
				Source:   src,
				Quotes:   qs,
				Renderer: rnd,
				Writer:   os.Stdout,
			}
			return run.Execute(cmd.Context(), spec, pipeline.ExecuteOptions{
				Columns:        cols,
				Filter:         f,
				PriceCacheTTL:  flagCacheTTL,
				PriceCacheSize: flagCacheSize,
				Concurrency:    flagConcurrency,
				Color:          !flagNoColor,
				PrettyJSON:     flagPrettyJSON,
			})
		},
	}

	rootCmd.Flags().StringVar(&flagSource, "source", "yaml", "data source: yaml|db")
	rootCmd.Flags().StringVar(&flagDBDSN, "db-dsn", "", "database DSN for db source")
	rootCmd.Flags().StringVar(&flagOutput, "output", "table", "output format: table|json")
	rootCmd.Flags().BoolVar(&flagNoColor, "no-color", false, "disable color output")
	rootCmd.Flags().BoolVar(&flagPrettyJSON, "pretty-json", false, "pretty-print JSON output")
	rootCmd.Flags().StringVar(&flagColumns, "columns", "", "comma-separated columns to display")
	rootCmd.Flags().StringVar(&flagFilter, "filter", "", "filter watchlists by name: substring (ci), name[,name...], glob, or /regex/")
	rootCmd.Flags().DurationVar(&flagCacheTTL, "price-cache-ttl", 5*time.Second, "price cache TTL")
	rootCmd.Flags().IntVar(&flagCacheSize, "price-cache-size", 1000, "price cache max size")
	rootCmd.Flags().IntVar(&flagConcurrency, "concurrency", 5, "quote fetch concurrency")
	rootCmd.Flags().BoolVar(&flagList, "list", false, "list watchlist names only")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
