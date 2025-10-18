//go:build !windows

package main

import (
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

func detectTerminalWidth() int {
	fd := int(os.Stdout.Fd())
	if ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ); err == nil && ws != nil {
		if ws.Col > 0 {
			return int(ws.Col)
		}
	}
	if cols, ok := os.LookupEnv("COLUMNS"); ok {
		if n, err := strconv.Atoi(cols); err == nil && n > 0 {
			return n
		}
	}
	return 0
}
