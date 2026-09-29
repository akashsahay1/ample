// Command ampls is the AMPLS command line interface (bin\ampls.exe).
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ampls/internal/core"
	"ampls/internal/paths"
)

// version is set at build time: -ldflags "-X main.version=1.2.3".
var version = "dev"

var homeFlag string

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "ampls",
		Short: "AMPLS - Apache, MySQL, PHP (Latest Software) for local development",
		Long: `AMPLS runs Apache, MySQL and multiple PHP versions for local PHP development.

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
	root.PersistentFlags().StringVar(&homeFlag, "home", "", "AMPLS data directory (overrides data-dir.txt and $AMPLS_HOME)")
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
	addMiscCommands(root)
	root.SetHelpCommandGroupID("misc")
	return root
}

func main() {
	core.Version = version
	initConsole()
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, red("error: ")+err.Error())
		os.Exit(1)
	}
}
