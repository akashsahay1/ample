// Command php-shim is installed as bin\php.exe. It picks the PHP version for
// the current directory (see internal/shim) and runs that php.exe with the same
// arguments, stdio and exit code.
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	"ampls/internal/paths"
	"ampls/internal/shim"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	minor, source, err := shim.ResolveVersion(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ampls php:", err)
		os.Exit(1)
	}
	exe := filepath.Join(paths.PHPDir(minor), paths.Exe("php"))
	if os.Getenv("AMPLS_SHIM_DEBUG") == "1" {
		fmt.Fprintf(os.Stderr, "ampls php: version %s (from %s) -> %s\n", minor, source, exe)
	}
	if _, err := os.Stat(exe); err != nil {
		fmt.Fprintf(os.Stderr, "ampls php: PHP %s (selected by %s) is not installed; run `ampls php:install %s`\n", minor, source, minor)
		os.Exit(1)
	}

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl+C reaches the child through the shared console; keep waiting for it.
	// (Notify rather than Ignore: ignored signals would be inherited on Unix.)
	signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "ampls php:", err)
		os.Exit(1)
	}
}
