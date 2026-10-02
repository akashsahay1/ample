package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"apnoro/internal/api"
	"apnoro/internal/php"
	"apnoro/internal/shim"
)

func addPHPCommands(root *cobra.Command) {
	root.AddCommand(
		&cobra.Command{
			Use:     "php:list",
			Aliases: []string{"php"},
			Short:   "List installed and available PHP versions",
			Long:    "List installed PHP versions and the versions available to install.\n\nExample:\n  apnoro php:list",
			GroupID: "php",
			Args:    cobra.NoArgs,
			RunE:    runPHPList,
		},
		&cobra.Command{
			Use:     "php:install <version>",
			Short:   "Download and install a PHP version",
			Long:    "Download and install a PHP version (NTS x64 from windows.php.net).\n\nExamples:\n  apnoro php:install 8.4\n  apnoro php:install 7.4",
			GroupID: "php",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ver := normMinor(args[0])
				for _, i := range installedPHP() {
					if i.Minor == ver {
						ok("PHP %s is already installed", i.Full)
						return nil
					}
				}
				fmt.Printf("Installing PHP %s\n", ver)
				bar := newProgressBar()
				err := backend().InstallPHP(ver, bar.update)
				bar.clear()
				if err != nil {
					return err
				}
				ok("PHP %s installed", ver)
				return nil
			},
		},
		&cobra.Command{
			Use:     "php:use <version>",
			Short:   "Set the default PHP version",
			Long:    "Set the default PHP version used by all sites that are not isolated, and by\nthe php command outside any site.\n\nExample:\n  apnoro php:use 8.3",
			GroupID: "php",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ver := normMinor(args[0])
				if err := backend().SetDefaultPHP(ver); err != nil {
					return err
				}
				ok("PHP %s is now the default", ver)
				return nil
			},
		},
		&cobra.Command{
			Use:     "php:remove <version>",
			Short:   "Uninstall a PHP version",
			Long:    "Remove an installed PHP version. It must not be the default or pinned by a site.\n\nExample:\n  apnoro php:remove 8.1",
			GroupID: "php",
			Args:    cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ver := normMinor(args[0])
				if err := backend().RemovePHP(ver); err != nil {
					return err
				}
				ok("PHP %s removed", ver)
				return nil
			},
		},
		&cobra.Command{
			Use:     "php:ini [version]",
			Short:   "Print the path of a PHP version's php.ini",
			Long:    "Print the php.ini path for a PHP version (default: the default version).\n\nExamples:\n  apnoro php:ini\n  notepad (apnoro php:ini 8.3)",
			GroupID: "php",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ver := normMinor(firstArg(args))
				if ver == "" {
					cfg, err := loadConfig()
					if err != nil {
						return err
					}
					ver = cfg.DefaultPHP
					if ver == "" {
						return fmt.Errorf("no PHP version is installed")
					}
				}
				p := backend().PHPIniPath(ver)
				if _, err := os.Stat(p); err != nil {
					return fmt.Errorf("PHP %s has no php.ini at %s (is it installed?)", ver, p)
				}
				fmt.Println(p)
				return nil
			},
		},
		&cobra.Command{
			Use:     "which-php",
			Short:   "Show which PHP version the php command uses here",
			Long:    "Resolve the PHP version for the current directory the same way the php\ncommand does (.apnoro-php file, site isolation, default).\n\nExample:\n  apnoro which-php",
			GroupID: "php",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				minor, source, err := shim.ResolveVersion(cwd)
				if err != nil {
					return err
				}
				w := newTable()
				row(w, "Version:", minor)
				row(w, "Source:", source)
				row(w, "Binary:", php.CLIPath(minor))
				return w.Flush()
			},
		},
	)
}

func installedPHP() []php.Installed {
	inst, _ := php.List()
	return inst
}

func runPHPList(cmd *cobra.Command, args []string) error {
	vs, err := backend().ListPHP()
	if err != nil {
		return err
	}
	if len(vs) == 0 {
		fmt.Println("No PHP versions installed or available (are you offline?).")
		return nil
	}
	w := newTable()
	row(w, "VERSION", "LATEST", "STATUS", "SITES", "NOTES")
	var avail []api.PHPVersion
	for _, v := range vs {
		if !v.Installed {
			avail = append(avail, v)
			continue
		}
		status := symOK + " installed"
		var notes []string
		if v.Default {
			notes = append(notes, "default")
		}
		if v.EOL {
			notes = append(notes, "end of life")
		}
		row(w, v.Version, v.Full, status, strconv.Itoa(v.SiteCount), strings.Join(notes, ", "))
	}
	for _, v := range avail {
		notes := humanBytes(v.DownloadSize)
		if v.EOL {
			if notes != "" {
				notes += ", "
			}
			notes += "end of life"
		}
		row(w, v.Version, v.Full, "available", "-", notes)
	}
	return w.Flush()
}
