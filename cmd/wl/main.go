package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

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

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
