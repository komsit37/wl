//go:build windows

package main

import (
	"os"
	"strconv"
)

func detectTerminalWidth() int {
	if cols, ok := os.LookupEnv("COLUMNS"); ok {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
