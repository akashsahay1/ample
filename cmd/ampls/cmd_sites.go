package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"ampls/internal/api"
	"ampls/internal/sites"
)

func addSiteCommands(root *cobra.Command) {
	root.AddCommand(
		&cobra.Command{
			Use:     "sites",
			Aliases: []string{"links"},
			Short:   "List all sites",
			Long:    "List every site served by AMPLS (parked folders and links).\nSites pinned to a PHP version are marked with *.\n\nExample:\n  ampls sites",
			GroupID: "sites",
			Args:    cobra.NoArgs,
			RunE:    runSites,
		},
		&cobra.Command{
			Use:     "park [dir]",
			Short:   "Serve every folder inside a directory as <folder>.test",
			Long:    "Park a directory: each sub-folder becomes a site named after the folder.\nDefaults to the current directory.\n\nExamples:\n  ampls park\n  ampls park D:\\Code",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir, err := absDir(firstArg(args))
				if err != nil {
					return err
				}
				if err := backend().Park(dir); err != nil {
					return err
				}
				ok("Parked %s", dir)
				return nil
			},
		},
		&cobra.Command{
			Use:     "unpark [dir]",
			Short:   "Stop serving a parked directory",
			Long:    "Remove a directory from the parked list. Defaults to the current directory.\n\nExamples:\n  ampls unpark\n  ampls unpark D:\\Code",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				dir, err := absDir(firstArg(args))
				if err != nil {
					return err
				}
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				found := false
				for _, p := range cfg.Parked {
					if strings.EqualFold(filepath.Clean(p), filepath.Clean(dir)) {
						found = true
					}
				}
				if !found {
					return fmt.Errorf("%s is not parked (see `ampls parked`)", dir)
				}
				if err := backend().Unpark(dir); err != nil {
					return err
				}
				ok("Unparked %s", dir)
				return nil
			},
		},
		&cobra.Command{
			Use:     "parked",
			Short:   "List parked directories",
			Long:    "List the parked directories.\n\nExample:\n  ampls parked",
			GroupID: "sites",
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cfg, err := loadConfig()
				if err != nil {
					return err
				}
				if len(cfg.Parked) == 0 {
					fmt.Println("No parked directories. Park one with `ampls park <dir>`.")
					return nil
				}
				for _, p := range cfg.Parked {
					fmt.Println(p)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:     "link [name]",
			Short:   "Serve the current directory as <name>.test",
			Long:    "Link the current directory as a site. The name defaults to the folder name\n(lowercased, spaces replaced by dashes).\n\nExamples:\n  ampls link\n  ampls link api",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				name := strings.ToLower(firstArg(args))
				if name == "" {
					name = sites.Slug(filepath.Base(cwd))
				}
				if err := backend().Link(name, cwd); err != nil {
					return err
				}
				if s, err := resolveSite(name); err == nil {
					ok("Linked %s -> %s", s.URL, cwd)
				} else {
					ok("Linked %s -> %s", name, cwd)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:     "unlink [name]",
			Short:   "Remove a linked site",
			Long:    "Remove a linked site. Defaults to the site for the current directory.\n\nExamples:\n  ampls unlink\n  ampls unlink api",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				name := firstArg(args)
				if name == "" {
					s, err := siteForCwd()
					if err != nil {
						return err
					}
					if !s.Linked {
						return fmt.Errorf("%s is served from a parked directory, not a link; use `ampls unpark` on its parent", s.Domain)
					}
					name = s.Name
				}
				if err := backend().Unlink(name); err != nil {
					return err
				}
				ok("Unlinked %s", name)
				return nil
			},
		},
		isolateCmd(),
		&cobra.Command{
			Use:     "unisolate [site]",
			Short:   "Make a site follow the default PHP version again",
			Long:    "Remove a site's pinned PHP version. Defaults to the site for the current directory.\n\nExamples:\n  ampls unisolate\n  ampls unisolate blog",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s, err := resolveSite(firstArg(args))
				if err != nil {
					return err
				}
				if err := backend().SetSitePHP(s.Name, ""); err != nil {
					return err
				}
				ok("%s now uses the default PHP version", s.Domain)
				return nil
			},
		},
		secureCmd(true),
		secureCmd(false),
		&cobra.Command{
			Use:     "open [site]",
			Short:   "Open a site in the browser",
			Long:    "Open a site in the default browser. Defaults to the site for the current directory.\n\nExamples:\n  ampls open\n  ampls open blog",
			GroupID: "sites",
			Args:    cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s, err := resolveSite(firstArg(args))
				if err != nil {
					return err
				}
				fmt.Println("Opening " + s.URL)
				return openURL(s.URL)
			},
		},
		newCmd(),
	)
}

func runSites(cmd *cobra.Command, args []string) error {
	ss, err := backend().ListSites()
	if err != nil {
		return err
	}
	if len(ss) == 0 {
		fmt.Println("No sites yet. Park a directory (`ampls park`) or link a folder (`ampls link`).")
		return nil
	}
	w := newTable()
	row(w, "SITE", "URL", "PHP", "HTTPS", "PATH")
	anyIso := false
	for _, s := range ss {
		ver := s.PHP
		if s.Isolated {
			ver += "*"
			anyIso = true
		}
		row(w, s.Name, s.URL, ver, yesNo(s.Secure), s.Path)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if anyIso {
		fmt.Println(dim("* isolated: pinned to a PHP version"))
	}
	return nil
}

func isolateCmd() *cobra.Command {
	var site string
	c := &cobra.Command{
		Use:   "isolate <version>",
		Short: "Pin a site to a PHP version",
		Long: `Pin the site containing the current directory (or --site) to a PHP version.

Examples:
  ampls isolate 8.2
  ampls isolate 7.4 --site legacy`,
		GroupID: "sites",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveSite(site)
			if err != nil {
				return err
			}
			ver := normMinor(args[0])
			if err := backend().SetSitePHP(s.Name, ver); err != nil {
				return err
			}
			ok("%s now uses PHP %s", s.Domain, ver)
			return nil
		},
	}
	c.Flags().StringVar(&site, "site", "", "site name (default: site for the current directory)")
	return c
}

func secureCmd(secure bool) *cobra.Command {
	use, short, long := "secure [site]", "Serve a site over HTTPS",
		"Serve a site over HTTPS with a certificate from the local AMPLS CA.\nDefaults to the site for the current directory.\n\nExamples:\n  ampls secure\n  ampls secure blog"
	if !secure {
		use, short, long = "unsecure [site]", "Serve a site over plain HTTP",
			"Stop serving a site over HTTPS. Defaults to the site for the current directory.\n\nExamples:\n  ampls unsecure\n  ampls unsecure blog"
	}
	return &cobra.Command{
		Use:     use,
		Short:   short,
		Long:    long,
		GroupID: "sites",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveSite(firstArg(args))
			if err != nil {
				return err
			}
			if err := backend().SetSiteSecure(s.Name, secure); err != nil {
				return err
			}
			if secure {
				ok("https://%s is secured", s.Domain)
				if o, err := backend().Overview(); err == nil && !o.CATrusted {
					warn("the AMPLS certificate authority is not trusted yet; run `ampls trust`")
				}
			} else {
				ok("http://%s is no longer secured", s.Domain)
			}
			return nil
		},
	}
}

func newCmd() *cobra.Command {
	var dir, phpVer string
	var db bool
	c := &cobra.Command{
		Use:   "new <laravel|wordpress|blank> <name>",
		Short: "Create a new Laravel, WordPress or blank PHP project",
		Long: `Create a new project and serve it as <name>.test.

The project is created in --dir (default: the first parked directory).
With --db a MySQL database named after the project is created and wired
into .env / wp-config.php.

Examples:
  ampls new laravel shop --db
  ampls new wordpress blog --db --php 8.3
  ampls new blank sandbox --dir D:\Code`,
		GroupID:   "sites",
		Args:      cobra.ExactArgs(2),
		ValidArgs: []string{api.KindLaravel, api.KindWordPress, api.KindBlank},
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := strings.ToLower(args[0])
			switch kind {
			case api.KindLaravel, api.KindWordPress, api.KindBlank:
			default:
				return fmt.Errorf("unknown project kind %q (expected laravel, wordpress or blank)", args[0])
			}
			req := api.NewProjectRequest{Name: strings.ToLower(args[1]), Kind: kind, PHP: normMinor(phpVer), CreateDB: db}
			if dir != "" {
				d, err := filepath.Abs(dir)
				if err != nil {
					return err
				}
				req.Directory = d
			}
			last := ""
			site, err := backend().NewProject(req, func(p api.Progress) {
				if p.Done || p.Message == "" || p.Message == last {
					return
				}
				last = p.Message
				if p.Percent >= 0 {
					fmt.Printf("  %s %s\n", dim(fmt.Sprintf("[%3.0f%%]", p.Percent)), p.Message)
				} else {
					fmt.Printf("  %s %s\n", dim("[ .. ]"), p.Message)
				}
			})
			if err != nil {
				return err
			}
			ok("%s is ready at %s", site.Name, site.URL)
			fmt.Println("  " + site.Path)
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "parent directory (default: first parked directory)")
	c.Flags().StringVar(&phpVer, "php", "", "PHP version for the site (default: the default version)")
	c.Flags().BoolVar(&db, "db", false, "create a MySQL database for the project")
	return c
}

func openURL(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.Command("open", url)
	default:
		c = exec.Command("xdg-open", url)
	}
	if err := c.Start(); err != nil {
		if runtime.GOOS == "windows" {
			return exec.Command("cmd", "/c", "start", "", url).Start()
		}
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
