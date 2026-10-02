// Command apnoro is the Apnoro command line interface (bin\apnoro.exe).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"apnoro/internal/core"
	"apnoro/internal/paths"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

// build is the build number (git commit count): -ldflags "-X main.build=42".
var build = ""

var homeFlag string

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "apnoro",
		Short: "Apnoro - Apache, PHP and MySQL for local development",
		Long: `Apnoro runs Apache, MySQL and multiple PHP versions for local PHP development.

Every folder inside a parked directory is served as http://<folder>.test;
individual folders can be linked, pinned to a PHP version and served over HTTPS.`,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if homeFlag != "" {
				paths.SetHome(homeFlag)
			}
		},
	}
	root.PersistentFlags().StringVar(&homeFlag, "home", "", "Apnoro data directory (overrides data-dir.txt and $APNORO_HOME)")
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddGroup(
		&cobra.Group{ID: "services", Title: "Services:"},
		&cobra.Group{ID: "sites", Title: "Sites:"},
		&cobra.Group{ID: "php", Title: "PHP:"},
		&cobra.Group{ID: "db", Title: "Databases:"},
		&cobra.Group{ID: "misc", Title: "Other:"},
	)
	addServiceCommands(root)
	addSiteCommands(root)
	addPHPCommands(root)
	addDBCommands(root)
	addEnvCommands(root)
	addMiscCommands(root)
	root.SetHelpCommandGroupID("misc")
	return root
}

func main() {
	core.Version = version
	core.Build = build
	initConsole()
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, red("error: ")+err.Error())
		os.Exit(1)
	}
}
