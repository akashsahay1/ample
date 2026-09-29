//go:build !windows

package main

import "os"

func initConsole() {
	colorOn = isTerminal() && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}
