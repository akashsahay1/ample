//go:build windows

// Command apnoro-helper is the Apnoro hosts-file helper Windows service. It runs
// as LocalSystem and applies validated requests from <home>\run\hosts.json.
//
//	apnoro-helper install --home <dir>   register + start the service (admin)
//	apnoro-helper uninstall              stop + remove the service (admin)
//	apnoro-helper run --home <dir>       service entry point (foreground when interactive)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"apnoro/internal/hosts"
)

const (
	serviceName = "ApnoroHelper"
	displayName = "Apnoro Helper"
	description = "Keeps the Apnoro block of the hosts file in sync with your local .test sites."
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: apnoro-helper install --home <dir> | uninstall | run --home <dir>")
	os.Exit(2)
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	home := fs.String("home", "", "Apnoro data directory")
	_ = fs.Parse(os.Args[2:])

	var err error
	switch cmd {
	case "install":
		err = install(*home)
	case "uninstall":
		err = uninstall()
	case "run":
		err = run(*home)
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "apnoro-helper:", err)
		os.Exit(1)
	}
}

// absHome validates --home. The service's --home is fixed in the SCM service
// config at install time (admin only); it must be an absolute local path.
func absHome(home string) (string, error) {
	if home == "" {
		return "", errors.New("--home is required")
	}
	h, err := filepath.Abs(home)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(h, `\\`) || filepath.VolumeName(h) == "" {
		return "", fmt.Errorf("--home must be a local absolute path, got %q", home)
	}
	return filepath.Clean(h), nil
}

func install(home string) error {
	home, err := absHome(home)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run as administrator): %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		// Already installed: stop, then point it at this binary/home.
		stopService(s)
		cfg, err := s.Config()
		if err != nil {
			s.Close()
			return err
		}
		cfg.BinaryPathName = windows.EscapeArg(exe) + " run --home " + windows.EscapeArg(home)
		cfg.StartType = mgr.StartAutomatic
		cfg.DisplayName = displayName
		cfg.Description = description
		cfg.ServiceStartName = "LocalSystem"
		if err := s.UpdateConfig(cfg); err != nil {
			s.Close()
			return fmt.Errorf("update service: %w", err)
		}
	} else {
		s, err = m.CreateService(serviceName, exe, mgr.Config{
			DisplayName:  displayName,
			Description:  description,
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
		}, "run", "--home", home)
		if err != nil {
			return fmt.Errorf("create service: %w", err)
		}
	}
	defer s.Close()
	_ = s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	}, 24*3600)
	if err := s.Start(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_ALREADY_RUNNING) {
		return fmt.Errorf("start service: %w", err)
	}
	fmt.Println("Apnoro Helper service installed and started.")
	return nil
}

func stopService(s *mgr.Service) {
	st, err := s.Control(svc.Stop)
	if err != nil {
		return
	}
	deadline := time.Now().Add(10 * time.Second)
	for st.State != svc.Stopped && time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)
		if st, err = s.Query(); err != nil {
			return
		}
	}
}

func uninstall() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect to service manager (run as administrator): %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(serviceName)
	if err != nil {
		return nil // not installed
	}
	defer s.Close()
	stopService(s)
	if err := s.Delete(); err != nil && !errors.Is(err, windows.ERROR_SERVICE_MARKED_FOR_DELETE) {
		return fmt.Errorf("delete service: %w", err)
	}
	fmt.Println("Apnoro Helper service removed.")
	return nil
}

// setupLog writes apnoro-helper.log next to the service binary (the admin-only
// install dir, e.g. C:\Program Files\Apnoro\bin). Not %ProgramData%\Apnoro:
// any user can pre-create that folder (ProgramData grants Users "create
// folders") and plant a link there, turning our LocalSystem log writes and the
// .old rename into an arbitrary-file write. If the binary's own directory were
// user-writable the service would be hijackable anyway.
func setupLog() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	p := filepath.Join(filepath.Dir(exe), "apnoro-helper.log")
	if st, err := os.Lstat(p); err == nil {
		if !st.Mode().IsRegular() {
			return // never follow a link
		}
		if st.Size() > 1<<20 {
			_ = os.Rename(p, p+".old")
		}
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		log.SetOutput(f)
	}
}

type handler struct{ home string }

func (h *handler) Execute(_ []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- hosts.RunHelperContext(ctx, h.home) }()
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				status <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
				}
				return false, 0
			}
		case err := <-done:
			cancel()
			if err != nil {
				log.Printf("helper stopped: %v", err)
				return true, 1
			}
			return false, 0
		}
	}
}

func run(home string) error {
	home, err := absHome(home)
	if err != nil {
		return err
	}
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if isSvc {
		setupLog()
		log.Printf("starting (home %s)", home)
		return svc.Run(serviceName, &handler{home: home})
	}
	// Interactive debugging: foreground until Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log.Printf("apnoro-helper running in foreground (home %s); Ctrl+C to stop", home)
	return hosts.RunHelperContext(ctx, home)
}
