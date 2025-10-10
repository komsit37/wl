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

			var items []map[string]any
			if err := yaml.Unmarshal(data, &items); err != nil {
				return fmt.Errorf("parse yaml %s: %w", filename, err)
			}

			// Determine all keys across items, put "sym" first if present, then others sorted.
			keySet := map[string]struct{}{}
			for _, m := range items {
				for k := range m {
					keySet[k] = struct{}{}
				}
			}
			var keys []string
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
