//go:build !windows

// Command ampls-helper is the Windows hosts helper service; it is not used on
// other operating systems.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "ampls-helper: not supported on this OS")
	os.Exit(1)
}
