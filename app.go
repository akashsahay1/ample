package main

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"ampls/internal/api"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is bound to the frontend. Its methods mirror api.Backend 1:1 plus a few
// UI helpers (dialogs, opening URLs/folders/terminals, window control).
type App struct {
	ctx      context.Context
	b        api.Backend
	quitting atomic.Bool
}

// NewApp wraps a Backend.
func NewApp(b api.Backend) *App { return &App{b: b} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if s, err := a.b.GetSettings(); err == nil && s.StartServicesOnLaunch {
		go func() {
			a.notifyError(a.b.StartAll())
			a.emitStatus()
		}()
	}
}

// NoticeTask is the progress task the frontend turns into an error toast for
// actions it did not start itself (launch-time start, tray menu).
const NoticeTask = "notice"

func (a *App) notifyError(err error) {
	if err != nil {
		a.emitProgress(api.Progress{Task: NoticeTask, Done: true, Percent: 100, Error: err.Error()})
	}
}

// phpMyAdminURL honours a non-default HTTP port.
func (a *App) phpMyAdminURL() string {
	if s, err := a.b.GetSettings(); err == nil && s.HTTPPort != 0 && s.HTTPPort != 80 {
		return fmt.Sprintf("http://localhost:%d/phpmyadmin", s.HTTPPort)
	}
	return "http://localhost/phpmyadmin"
}

func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	if a.quitting.Load() || !trayAvailable {
		return false
	}
	runtime.WindowHide(ctx)
	return true
}

func (a *App) shutdown(ctx context.Context) {
	if s, err := a.b.GetSettings(); err == nil && s.StopServicesOnQuit {
		_ = a.b.StopAll()
	}
	stopTray()
}

func (a *App) emitStatus() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, api.StatusEvent)
	}
}

func (a *App) emitProgress(p api.Progress) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, api.ProgressEvent, p)
	}
}

// mutated emits a status event and passes the error through.
func (a *App) mutated(err error) error {
	a.emitStatus()
	return err
}

// ---- api.Backend mirror ----

func (a *App) Overview() (api.Overview, error) { return a.b.Overview() }

func (a *App) StartAll() error                  { return a.mutated(a.b.StartAll()) }
func (a *App) StopAll() error                   { return a.mutated(a.b.StopAll()) }
func (a *App) RestartAll() error                { return a.mutated(a.b.RestartAll()) }
func (a *App) StartService(name string) error   { return a.mutated(a.b.StartService(name)) }
func (a *App) StopService(name string) error    { return a.mutated(a.b.StopService(name)) }
func (a *App) RestartService(name string) error { return a.mutated(a.b.RestartService(name)) }

func (a *App) ListSites() ([]api.Site, error) { return a.b.ListSites() }
func (a *App) Park(dir string) error          { return a.mutated(a.b.Park(dir)) }
func (a *App) Unpark(dir string) error        { return a.mutated(a.b.Unpark(dir)) }
func (a *App) Link(name, path string) error   { return a.mutated(a.b.Link(name, path)) }
func (a *App) Unlink(name string) error       { return a.mutated(a.b.Unlink(name)) }
func (a *App) SetSitePHP(site, version string) error {
	return a.mutated(a.b.SetSitePHP(site, version))
}
func (a *App) SetSiteSecure(site string, secure bool) error {
	return a.mutated(a.b.SetSiteSecure(site, secure))
}

// NewProject creates a project; progress is emitted as api.ProgressEvent with
// task "project:<name>".
func (a *App) NewProject(req api.NewProjectRequest) (api.Site, error) {
	s, err := a.b.NewProject(req, a.emitProgress)
	if err != nil {
		a.emitProgress(api.Progress{Task: "project:" + req.Name, Done: true, Error: err.Error(), Percent: 100})
	}
	a.emitStatus()
	return s, err
}

func (a *App) ListPHP() ([]api.PHPVersion, error) { return a.b.ListPHP() }

// InstallPHP installs a PHP minor; progress is emitted as api.ProgressEvent
// with task "php:install:<version>".
func (a *App) InstallPHP(version string) error {
	err := a.b.InstallPHP(version, a.emitProgress)
	if err != nil {
		a.emitProgress(api.Progress{Task: "php:install:" + version, Done: true, Error: err.Error()})
	}
	return a.mutated(err)
}
func (a *App) RemovePHP(version string) error     { return a.mutated(a.b.RemovePHP(version)) }
func (a *App) SetDefaultPHP(version string) error { return a.mutated(a.b.SetDefaultPHP(version)) }
func (a *App) GetPHPSettings(version string) (api.PHPSettings, error) {
	return a.b.GetPHPSettings(version)
}
func (a *App) SavePHPSettings(s api.PHPSettings) error { return a.mutated(a.b.SavePHPSettings(s)) }
func (a *App) PHPIniPath(version string) string        { return a.b.PHPIniPath(version) }

func (a *App) MySQLInfo() (api.MySQLInfo, error)      { return a.b.MySQLInfo() }
func (a *App) ListDatabases() ([]api.Database, error) { return a.b.ListDatabases() }
func (a *App) CreateDatabase(name string) error       { return a.mutated(a.b.CreateDatabase(name)) }
func (a *App) DropDatabase(name string) error         { return a.mutated(a.b.DropDatabase(name)) }
func (a *App) ImportSQL(database, file string) error {
	return a.mutated(a.b.ImportSQL(database, file))
}
func (a *App) ExportDatabase(database, file string) error { return a.b.ExportDatabase(database, file) }
func (a *App) SetMySQLPassword(password string) error {
	return a.mutated(a.b.SetMySQLPassword(password))
}

func (a *App) LogNames() []string                             { return a.b.LogNames() }
func (a *App) ReadLog(name string, lines int) (string, error) { return a.b.ReadLog(name, lines) }

func (a *App) GetSettings() (api.Settings, error) { return a.b.GetSettings() }
func (a *App) SaveSettings(s api.Settings) error  { return a.mutated(a.b.SaveSettings(s)) }

func (a *App) TrustCA() error { return a.mutated(a.b.TrustCA()) }

// ---- api.Coexistence (optional; checked by type assertion) ----

var errNoCoexistence = errors.New("importing and detecting other environments is not supported by this backend")

func (a *App) coexist() (api.Coexistence, error) {
	if c, ok := a.b.(api.Coexistence); ok {
		return c, nil
	}
	return nil, errNoCoexistence
}

// DetectEnvironments lists other local stacks (XAMPP, Herd, Laragon, WAMP).
func (a *App) DetectEnvironments() ([]api.ExternalEnv, error) {
	c, err := a.coexist()
	if err != nil {
		return nil, err
	}
	return c.DetectEnvironments()
}

// PortConflicts lists ports AMPLS is configured to use that another program holds.
func (a *App) PortConflicts() ([]api.PortConflict, error) {
	c, err := a.coexist()
	if err != nil {
		return nil, err
	}
	return c.PortConflicts()
}

// StopEnvironment stops another environment's own servers (user-initiated).
func (a *App) StopEnvironment(kind string) error {
	c, err := a.coexist()
	if err != nil {
		return err
	}
	return a.mutated(c.StopEnvironment(kind))
}

// ScanImport previews what an import from kind would bring over. src is only
// used for api.EnvMySQL (and Herd Pro databases).
func (a *App) ScanImport(kind string, src *api.MySQLSource) (api.ImportPlan, error) {
	c, err := a.coexist()
	if err != nil {
		return api.ImportPlan{}, err
	}
	return c.ScanImport(kind, src)
}

// RunImport performs an import; progress is emitted as api.ProgressEvent with
// task api.ImportTaskPrefix+kind.
func (a *App) RunImport(req api.ImportRequest) error {
	c, err := a.coexist()
	if err == nil {
		err = c.RunImport(req, a.emitProgress)
	}
	if err != nil {
		a.emitProgress(api.Progress{Task: api.ImportTaskPrefix + req.Kind, Done: true, Percent: 100, Error: err.Error()})
	}
	return a.mutated(err)
}

// ---- UI helpers ----

// AppVersion returns the desktop app version.
// AppVersion is shown in Settings › About, e.g. "1.0.0 (build 42)".
func (a *App) AppVersion() string {
	if build != "" {
		return version + " (build " + build + ")"
	}
	return version
}

// OpenURL opens a URL in the default browser.
func (a *App) OpenURL(url string) { runtime.BrowserOpenURL(a.ctx, url) }

// OpenFolder reveals a folder (or file) in the OS file manager.
func (a *App) OpenFolder(path string) error { return openFolder(path) }

// OpenPath opens a file with its associated application (e.g. php.ini in an editor).
func (a *App) OpenPath(path string) error { return openPath(path) }

// OpenTerminal opens a terminal in dir.
func (a *App) OpenTerminal(dir string) error { return openTerminal(dir) }

// SelectDirectory shows a folder picker; "" when cancelled.
func (a *App) SelectDirectory(title string) (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: title, CanCreateDirectories: true})
}

// SelectFile shows a file picker filtered by pattern (e.g. "*.sql;*.sql.gz"); "" when cancelled.
func (a *App) SelectFile(title, pattern string) (string, error) {
	opts := runtime.OpenDialogOptions{Title: title}
	if pattern != "" {
		opts.Filters = []runtime.FileFilter{{DisplayName: pattern, Pattern: pattern}, {DisplayName: "All files", Pattern: "*.*"}}
	}
	return runtime.OpenFileDialog(a.ctx, opts)
}

// SaveFile shows a save dialog; "" when cancelled.
func (a *App) SaveFile(title, defaultName string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: title, DefaultFilename: defaultName})
}

// HideWindow hides the window to the tray (or minimises when no tray exists).
func (a *App) HideWindow() {
	if trayAvailable {
		runtime.WindowHide(a.ctx)
	} else {
		runtime.WindowMinimise(a.ctx)
	}
}

// ShowWindow brings the window to the front.
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
}

// Quit exits the application for real (bypasses hide-to-tray).
func (a *App) Quit() {
	a.quitting.Store(true)
	runtime.Quit(a.ctx)
}
