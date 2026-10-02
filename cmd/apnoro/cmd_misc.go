package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"apnoro/internal/certs"
	"apnoro/internal/core"
	"apnoro/internal/hosts"
	"apnoro/internal/paths"
)

func addMiscCommands(root *cobra.Command) {
	var lines int
	logs := &cobra.Command{
		Use:   "logs [name]",
		Short: "Show a log file (lists log names when none given)",
		Long: `Print the last lines of an Apnoro log. Without a name, list the available logs.

Examples:
  apnoro logs
  apnoro logs apache-error
  apnoro logs php-8.3 -n 50`,
		GroupID: "misc",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Println("Available logs:")
				for _, n := range backend().LogNames() {
					fmt.Println("  " + n)
				}
				fmt.Println(dim("\nShow one with `apnoro logs <name>`. Files are in " + paths.LogsDir()))
				return nil
			}
			out, err := backend().ReadLog(args[0], lines)
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Println(dim("(log is empty)"))
				return nil
			}
			fmt.Println(out)
			return nil
		},
	}
	logs.Flags().IntVarP(&lines, "lines", "n", 100, "number of lines to show")

	var (
		setupOpts   core.SetupOptions
		mysqlPwFile string
	)
	setup := &cobra.Command{
		Use:   "setup",
		Short: "Prepare the data directory (run by the installer)",
		Long: `Prepare a freshly installed or upgraded Apnoro data directory: PHP ini files,
MySQL data directory, local certificate authority, Apache config and hosts
entries. Safe to run repeatedly. The installer runs it elevated.

Examples:
  apnoro setup
  apnoro setup --home D:\Apnoro --park-default --trust-ca-machine`,
		GroupID: "misc",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Also log to <Home>\logs\setup.log: the installer runs this hidden, so
			// the file is the only place a failure can be read afterwards.
			var logf *os.File
			if err := os.MkdirAll(paths.LogsDir(), 0o755); err == nil {
				logf, _ = os.OpenFile(filepath.Join(paths.LogsDir(), "setup.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
			}
			log := func(s string) {
				fmt.Println(s)
				if logf != nil {
					fmt.Fprintf(logf, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), s)
				}
			}
			log(fmt.Sprintf("apnoro %s setup (home %s)", version, paths.Home()))
			if mysqlPwFile != "" {
				// One-time file written by the installer; read and delete it so the
				// password never sits on a command line or stays on disk.
				b, err := os.ReadFile(mysqlPwFile)
				_ = os.Remove(mysqlPwFile)
				if err != nil {
					log("error: read MySQL password file: " + err.Error())
					return err
				}
				// Inno Setup writes UTF-8 with a BOM.
				setupOpts.MySQLPassword = strings.TrimRight(strings.TrimPrefix(string(b), string(rune(0xFEFF))), "\r\n")
			}
			err := backend().Setup(setupOpts, log)
			if err != nil {
				log("error: " + err.Error())
			}
			if logf != nil {
				logf.Close()
			}
			return err
		},
	}
	setup.Flags().BoolVar(&setupOpts.ParkDefault, "park-default", false, "create and park ~/Apnoro/Sites")
	setup.Flags().BoolVar(&setupOpts.TrustCAMachine, "trust-ca-machine", false, "trust the Apnoro CA machine-wide (requires admin)")
	setup.Flags().StringVar(&mysqlPwFile, "mysql-password-file", "", "file holding the root password for a fresh MySQL install (deleted after reading)")

	var trustMachine bool
	trust := &cobra.Command{
		Use:   "trust",
		Short: "Trust the Apnoro certificate authority for HTTPS sites",
		Long: "Create the local Apnoro certificate authority if needed and add it to the\ncurrent user's trusted root store, so browsers accept secured sites.\n\n" +
			"--machine adds the existing CA to the machine store instead (requires admin;\nthe installer uses it). It never creates a CA, so the key stays owned by the user.\n\nExamples:\n  apnoro trust\n  apnoro trust --machine",
		GroupID: "misc",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if trustMachine {
				if err := certs.TrustCA(true); err != nil {
					return err
				}
			} else if err := backend().TrustCA(); err != nil {
				return err
			}
			ok("Apnoro certificate authority trusted")
			return nil
		},
	}
	trust.Flags().BoolVar(&trustMachine, "machine", false, "trust the existing CA machine-wide (requires admin)")

	hostsCmd := &cobra.Command{
		Use:     "hosts",
		Short:   "Manage the Apnoro block in the system hosts file",
		Long:    "Manage the Apnoro-managed block in " + hosts.HostsPath() + ".\nApnoro normally updates it automatically.",
		GroupID: "misc",
	}
	hostsCmd.AddCommand(
		&cobra.Command{
			Use:   "apply",
			Short: "Apply the pending hosts request (requires admin)",
			Long:  "Apply the domains requested in run/hosts.json to the hosts file. Apnoro runs\nthis elevated automatically when the helper service is unavailable.\n\nExample:\n  apnoro hosts apply",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := hosts.ApplyPending(); err != nil {
					return err
				}
				ok("Hosts file updated")
				return nil
			},
		},
		&cobra.Command{
			Use:   "clear",
			Short: "Remove all Apnoro entries from the hosts file (requires admin)",
			Long:  "Remove the Apnoro block from the hosts file.\n\nExample:\n  apnoro hosts clear",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := hosts.Apply(nil); err != nil {
					return err
				}
				ok("Apnoro entries removed from the hosts file")
				return nil
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List the domains in the Apnoro hosts block",
			Long:  "List the domains currently in the Apnoro-managed hosts block.\n\nExample:\n  apnoro hosts list",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				ds, err := hosts.Current()
				if err != nil {
					return err
				}
				if len(ds) == 0 {
					fmt.Println("No Apnoro entries in " + hosts.HostsPath())
					return nil
				}
				for _, d := range ds {
					fmt.Println(d)
				}
				return nil
			},
		},
	)

	root.AddCommand(
		logs,
		trust,
		setup,
		hostsCmd,
		&cobra.Command{
			Use:     "version",
			Short:   "Print the Apnoro version",
			Long:    "Print the Apnoro version and data directory.\n\nExample:\n  apnoro version",
			GroupID: "misc",
			Args:    cobra.NoArgs,
			Run: func(cmd *cobra.Command, args []string) {
				if build != "" {
					fmt.Printf("apnoro %s build %s (%s/%s)\n", version, build, runtime.GOOS, runtime.GOARCH)
				} else {
					fmt.Printf("apnoro %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
				}
				fmt.Println("home: " + paths.Home())
			},
		},
	)
}
