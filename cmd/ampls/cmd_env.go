package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"ampls/internal/api"
)

func addEnvCommands(root *cobra.Command) {
	root.AddGroup(&cobra.Group{ID: "env", Title: "Other stacks (XAMPP, Herd, Laragon, WAMP):"})

	env := &cobra.Command{
		Use:     "env",
		Short:   "Show other local stacks and port conflicts",
		Long:    "List XAMPP, Laravel Herd, Laragon and WAMP installs found on this machine,\nand any program holding a port AMPLS needs.\n\nExample:\n  ampls env",
		GroupID: "env",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b := backend()
			envs, err := b.DetectEnvironments()
			if err != nil {
				return err
			}
			if len(envs) == 0 {
				fmt.Println("No other local stacks found.")
			} else {
				w := newTable()
				row(w, bold("STACK"), bold("STATE"), bold("PORTS"), bold("PHP"), bold("SITES"), bold("PATH"))
				for _, e := range envs {
					state := dim("stopped")
					if e.Running {
						state = green("running")
					}
					ports := make([]string, 0, len(e.Ports))
					for _, p := range e.Ports {
						ports = append(ports, strconv.Itoa(p))
					}
					phpv := e.PHP
					if e.OnPath {
						phpv += " (on PATH)"
					}
					row(w, e.Name, state, strings.Join(ports, ","), phpv, strconv.Itoa(e.Sites), e.Path)
				}
				w.Flush()
				for _, e := range envs {
					for _, n := range e.Notes {
						fmt.Println(dim("  " + e.Name + ": " + n))
					}
				}
			}
			cs, err := b.PortConflicts()
			if err != nil {
				return err
			}
			if len(cs) == 0 {
				fmt.Println()
				ok("No port conflicts")
				return nil
			}
			fmt.Println()
			for _, c := range cs {
				who := c.Process
				if c.Path != "" {
					who += " (" + c.Path + ")"
				}
				warn("port %d (%s) is in use by %s", c.Port, c.Service, who)
				if c.Env != "" && c.Env != "ampls" {
					fmt.Println(dim(fmt.Sprintf("  run `ampls env:stop %s`, or change the AMPLS port in Settings", c.Env)))
				}
			}
			return nil
		},
	}

	var force bool
	stop := &cobra.Command{
		Use:       "env:stop <xampp|herd|laragon|wamp>",
		Short:     "Stop another stack's servers so AMPLS can use the ports",
		Long:      "Stop the web and database servers of another local stack. Only processes\nrunning from that stack's own folders are stopped; nothing is uninstalled.\n\nExample:\n  ampls env:stop xampp",
		GroupID:   "env",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{api.EnvXAMPP, api.EnvHerd, api.EnvLaragon, api.EnvWAMP},
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := strings.ToLower(args[0])
			if !force && !confirm(fmt.Sprintf("Stop %s's servers?", kind)) {
				return errors.New("aborted")
			}
			if err := backend().StopEnvironment(kind); err != nil {
				return err
			}
			ok("%s stopped", kind)
			return nil
		},
	}
	stop.Flags().BoolVarP(&force, "force", "f", false, "do not ask for confirmation")

	var (
		dryRun, park, allSites, allDB, installPHP, secure, overwrite bool
		sitesFlag, dbFlag                                           []string
		src                                                         api.MySQLSource
	)
	imp := &cobra.Command{
		Use:   "import <xampp|herd|laragon|wamp|mysql>",
		Short: "Import projects and databases from another stack",
		Long: `Import sites and databases from XAMPP, Laravel Herd, Laragon, WAMP or any
MySQL/MariaDB server. Project folders are served in place (never copied), and
the other stack is not changed.

Without --sites or --park every site without a conflict is linked.
The MySQL password is read from $AMPLS_IMPORT_PASSWORD, never from a flag.

Examples:
  ampls import herd --dry-run
  ampls import herd --install-php --secure
  ampls import xampp --park --all-db
  ampls import mysql --port 3307 --user root --db shop,blog`,
		GroupID:   "env",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{api.EnvXAMPP, api.EnvHerd, api.EnvLaragon, api.EnvWAMP, api.EnvMySQL},
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := strings.ToLower(args[0])
			var srcp *api.MySQLSource
			if kind == api.EnvMySQL || cmd.Flags().Changed("port") || cmd.Flags().Changed("user") || cmd.Flags().Changed("host") {
				src.Password = os.Getenv("AMPLS_IMPORT_PASSWORD")
				srcp = &src
			}
			b := backend()
			plan, err := b.ScanImport(kind, srcp)
			if err != nil {
				return err
			}
			printPlan(plan)
			if dryRun {
				return nil
			}

			req := api.ImportRequest{Kind: kind, InstallPHP: installPHP, KeepSecure: secure, Overwrite: overwrite, MySQL: srcp}
			if park {
				req.ParkDirs = plan.ParkedDirs
			}
			want := map[string]bool{}
			for _, s := range sitesFlag {
				want[strings.ToLower(s)] = true
			}
			for _, s := range plan.Sites {
				if s.Conflict != "" {
					continue
				}
				pick := allSites || want[strings.ToLower(s.Name)] || want[strings.ToLower(s.Path)]
				if len(sitesFlag) == 0 && !park {
					pick = true
				}
				if pick {
					req.Sites = append(req.Sites, s.Path)
				}
			}
			wantDB := map[string]bool{}
			for _, d := range dbFlag {
				wantDB[d] = true
			}
			for _, d := range plan.Databases {
				if allDB || wantDB[d.Name] {
					req.Databases = append(req.Databases, d.Name)
				}
			}
			if len(req.Sites) == 0 && len(req.ParkDirs) == 0 && len(req.Databases) == 0 {
				return errors.New("nothing selected to import")
			}
			fmt.Printf("\nImporting %d site(s), %d parked folder(s), %d database(s)\n", len(req.Sites), len(req.ParkDirs), len(req.Databases))
			bar := newProgressBar()
			err = b.RunImport(req, func(p api.Progress) { bar.update(p) })
			bar.clear()
			if err != nil {
				return err
			}
			ok("Import finished")
			return nil
		},
	}
	f := imp.Flags()
	f.BoolVar(&dryRun, "dry-run", false, "only show what would be imported")
	f.BoolVar(&park, "park", false, "park the stack's site folders (Herd parked paths, XAMPP htdocs)")
	f.StringSliceVar(&sitesFlag, "sites", nil, "sites to link, by name or path")
	f.BoolVar(&allSites, "all-sites", false, "link every site without a conflict")
	f.StringSliceVar(&dbFlag, "db", nil, "databases to copy")
	f.BoolVar(&allDB, "all-db", false, "copy every database")
	f.BoolVar(&installPHP, "install-php", false, "install PHP versions sites are pinned to and keep the pins")
	f.BoolVar(&secure, "secure", false, "serve sites over HTTPS that were secured in the source")
	f.BoolVar(&overwrite, "overwrite", false, "replace AMPLS databases with the same name")
	f.StringVar(&src.Host, "host", "127.0.0.1", "source MySQL host")
	f.IntVar(&src.Port, "port", 3306, "source MySQL port")
	f.StringVar(&src.User, "user", "root", "source MySQL user")

	root.AddCommand(env, stop, imp)
}

func printPlan(p api.ImportPlan) {
	fmt.Println(bold("Import from " + p.Source))
	for _, d := range p.ParkedDirs {
		fmt.Println("  parked folder: " + d)
	}
	if len(p.Sites) > 0 {
		w := newTable()
		row(w, bold("  SITE"), bold("PHP"), bold("HTTPS"), bold("SOURCE"), bold("FOLDER"))
		for _, s := range p.Sites {
			name := "  " + s.Name
			if s.Conflict != "" {
				name = "  " + yellow(s.Name+" (skip: "+s.Conflict+")")
			}
			phpv := s.PHP
			if phpv == "" {
				phpv = dim("default")
			}
			row(w, name, phpv, yesNo(s.Secure), s.Source, s.Path)
		}
		w.Flush()
	}
	for _, d := range p.Databases {
		extra := ""
		if d.Exists {
			extra = yellow(" (exists in AMPLS)")
		}
		fmt.Printf("  database: %s %s%s\n", d.Name, dim(humanBytes(d.SizeBytes)), extra)
	}
	if len(p.MissingPHP) > 0 {
		fmt.Println(dim("  PHP not installed in AMPLS: " + strings.Join(p.MissingPHP, ", ") + " (use --install-php)"))
	}
	for _, n := range p.Notes {
		fmt.Println(dim("  " + n))
	}
}
