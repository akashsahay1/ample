package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"ampls/internal/api"
)

var serviceNames = []string{api.ServiceApache, api.ServiceMySQL}

func serviceTitle(name string) string {
	switch name {
	case api.ServiceApache:
		return "Apache"
	case api.ServiceMySQL:
		return "MySQL"
	}
	return name
}

func checkService(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	n := strings.ToLower(args[0])
	for _, s := range serviceNames {
		if n == s {
			return n, nil
		}
	}
	return "", fmt.Errorf("unknown service %q (expected apache or mysql)", args[0])
}

func serviceCmd(use, short, long, verb string, one func(string) error, all func() error) *cobra.Command {
	return &cobra.Command{
		Use:       use + " [apache|mysql]",
		Short:     short,
		Long:      long,
		GroupID:   "services",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: serviceNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := checkService(args)
			if err != nil {
				return err
			}
			if name != "" {
				if err := one(name); err != nil {
					return fmt.Errorf("%s: %w", serviceTitle(name), err)
				}
				ok("%s %s", serviceTitle(name), verb)
				return nil
			}
			if err := all(); err != nil {
				return err
			}
			ok("Apache and MySQL %s", verb)
			return nil
		},
	}
}

func addServiceCommands(root *cobra.Command) {
	root.AddCommand(
		serviceCmd("start", "Start Apache and MySQL (or one service)",
			`Start the AMPLS services in the background. They keep running after
the command exits.

Examples:
  ampls start
  ampls start mysql`, "started",
			func(n string) error { return backend().StartService(n) }, func() error { return backend().StartAll() }),
		serviceCmd("stop", "Stop Apache and MySQL (or one service)",
			`Stop the AMPLS services.

Examples:
  ampls stop
  ampls stop apache`, "stopped",
			func(n string) error { return backend().StopService(n) }, func() error { return backend().StopAll() }),
		serviceCmd("restart", "Restart Apache and MySQL (or one service)",
			`Restart the AMPLS services (e.g. after editing php.ini by hand).

Examples:
  ampls restart
  ampls restart apache`, "restarted",
			func(n string) error { return backend().RestartService(n) }, func() error { return backend().RestartAll() }),
		&cobra.Command{
			Use:     "status",
			Short:   "Show service status, default PHP and data directory",
			Long:    "Show whether Apache and MySQL are running, their PIDs, versions and ports.\n\nExample:\n  ampls status",
			GroupID: "services",
			Args:    cobra.NoArgs,
			RunE:    runStatus,
		},
	)
}

func runStatus(cmd *cobra.Command, args []string) error {
	o, err := backend().Overview()
	if err != nil {
		return err
	}
	w := newTable()
	row(w, "SERVICE", "STATE", "PID", "VERSION", "PORTS")
	for _, s := range o.Services {
		state, pid := symFail+" stopped", "-"
		if s.Running {
			state, pid = symOK+" running", strconv.Itoa(s.PID)
		}
		ver := s.Version
		if ver == "" {
			ver = "not installed"
		}
		var ports []string
		for _, p := range s.Ports {
			ports = append(ports, strconv.Itoa(p))
		}
		row(w, serviceTitle(s.Name), state, pid, ver, strings.Join(ports, ", "))
	}
	w.Flush()
	fmt.Println()
	def := o.DefaultPHP
	if def == "" {
		def = "none installed (run `ampls php:install 8.4`)"
	}
	w = newTable()
	row(w, "Default PHP:", def)
	if len(o.PHPVersions) > 0 {
		row(w, "Installed PHP:", strings.Join(o.PHPVersions, ", "))
	}
	row(w, "Sites:", strconv.Itoa(o.SiteCount))
	ca := symOK + " trusted"
	if !o.CATrusted {
		ca = symFail + " not trusted (run `ampls trust`)"
	}
	row(w, "HTTPS CA:", ca)
	row(w, "Home:", o.Home)
	row(w, "Version:", o.AppVersion)
	return w.Flush()
}
