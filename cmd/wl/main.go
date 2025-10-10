package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "wl <file.yaml>",
		Short: "Render a YAML watchlist to a table",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("requires exactly 1 YAML file argument")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			viper.AutomaticEnv()

			filename := args[0]
			f, err := os.Open(filename)
			if err != nil {
				return fmt.Errorf("open %s: %w", filename, err)
			}
			defer f.Close()

			data, err := io.ReadAll(f)
			if err != nil {
				return fmt.Errorf("read %s: %w", filename, err)
			}

			// Support two YAML shapes:
			// 1) Top-level list of items: "- sym: ..."
			// 2) Map with optional columns and items: "columns: [...]; items: [...]"
			var (
				items []map[string]any
				keys  []string
			)

			// Try list form: top-level list of maps
			if err := yaml.Unmarshal(data, &items); err != nil {
				// Try map form: object with columns + items
				var alt struct {
					Columns []string         `yaml:"columns"`
					Items   []map[string]any `yaml:"items"`
				}
				if err2 := yaml.Unmarshal(data, &alt); err2 != nil {
					return fmt.Errorf("parse yaml %s: %w", filename, err)
				}
				items = alt.Items
				// If columns provided, respect order; otherwise compute fallback below.
				if len(alt.Columns) > 0 {
					keys = append(keys, alt.Columns...)
				}
			}

			// If keys not set yet, compute from items with sensible ordering.
			if len(keys) == 0 {
				// Determine all keys across items, put "sym" first if present, then others sorted.
				keySet := map[string]struct{}{}
				for _, m := range items {
					for k := range m {
						keySet[k] = struct{}{}
					}
				}
				if _, ok := keySet["sym"]; ok {
					keys = append(keys, "sym")
					delete(keySet, "sym")
				}
				var rest []string
				for k := range keySet {
					rest = append(rest, k)
				}
				sort.Strings(rest)
				keys = append(keys, rest...)
			}

			tw := table.NewWriter()
			tw.SetOutputMirror(os.Stdout)
			// Header
			hdr := make(table.Row, len(keys))
			for i, k := range keys {
				hdr[i] = strings.ToUpper(k)
			}
			tw.AppendHeader(hdr)

			// Rows
			for _, m := range items {
				row := make(table.Row, len(keys))
				for i, k := range keys {
					if v, ok := m[k]; ok && v != nil {
						row[i] = fmt.Sprint(v)
					} else {
						row[i] = ""
					}
				}
				tw.AppendRow(row)
			}

			tw.Render()
			return nil
		},
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
