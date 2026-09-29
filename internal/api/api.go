// Package api defines the contract between the GUI/CLI and the AMPLS core.
// The GUI binds to an implementation of Backend (internal/core in production,
// internal/api/mock for frontend development).
package api

// Service names.
const (
	ServiceApache = "apache"
	ServiceMySQL  = "mysql"
)

// Project kinds for NewProject.
const (
	KindLaravel   = "laravel"
	KindWordPress = "wordpress"
	KindBlank     = "blank"
)

// ProgressEvent is the Wails event name carrying Progress payloads.
const ProgressEvent = "ampls:progress"

// StatusEvent is emitted (no payload) whenever service/site state changes.
const StatusEvent = "ampls:status"

type ServiceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
	Ports   []int  `json:"ports"`
	Error   string `json:"error"`
}

type Overview struct {
	Services    []ServiceStatus `json:"services"`
	DefaultPHP  string          `json:"defaultPhp"`
	PHPVersions []string        `json:"phpVersions"` // installed minors, newest first
	SiteCount   int             `json:"siteCount"`
	Home        string          `json:"home"`
	AppVersion  string          `json:"appVersion"`
	CATrusted   bool            `json:"caTrusted"`
}

type Site struct {
	Name      string `json:"name"`     // "blog"
	Domain    string `json:"domain"`   // "blog.test"
	URL       string `json:"url"`      // "https://blog.test"
	Path      string `json:"path"`     // project folder
	DocRoot   string `json:"docRoot"`  // served folder (path/public for Laravel)
	PHP       string `json:"php"`      // effective PHP minor
	Isolated  bool   `json:"isolated"` // PHP pinned (not following default)
	Secure    bool   `json:"secure"`
	Linked    bool   `json:"linked"`    // via Link (not a parked dir)
	Framework string `json:"framework"` // "laravel" | "wordpress" | "php"
}

type PHPVersion struct {
	Version      string `json:"version"` // minor, "8.3"
	Full         string `json:"full"`    // "8.3.12" (installed or latest available)
	Installed    bool   `json:"installed"`
	Default      bool   `json:"default"`
	EOL          bool   `json:"eol"`
	SiteCount    int    `json:"siteCount"`
	DownloadSize int64  `json:"downloadSize"`
}

type Extension struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type PHPSettings struct {
	Version    string            `json:"version"`
	Ini        map[string]string `json:"ini"` // memory_limit, upload_max_filesize, post_max_size, max_execution_time, display_errors
	Extensions []Extension       `json:"extensions"`
}

type MySQLInfo struct {
	Running  bool   `json:"running"`
	Version  string `json:"version"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	DataDir  string `json:"dataDir"`
}

type Database struct {
	Name      string `json:"name"`
	Tables    int    `json:"tables"`
	SizeBytes int64  `json:"sizeBytes"`
}

type NewProjectRequest struct {
	Name      string `json:"name"`      // folder + site name, e.g. "blog"
	Kind      string `json:"kind"`      // KindLaravel | KindWordPress | KindBlank
	Directory string `json:"directory"` // parent dir, normally a parked dir
	PHP       string `json:"php"`       // "" = default
	CreateDB  bool   `json:"createDb"`  // create a MySQL database named after the project
}

type Progress struct {
	Task    string  `json:"task"` // stable id, e.g. "php:install:8.2" or "project:blog"
	Message string  `json:"message"`
	Percent float64 `json:"percent"` // 0..100, -1 = indeterminate
	Done    bool    `json:"done"`
	Error   string  `json:"error"`
}

type ProgressFunc func(Progress)

type Settings struct {
	TLD                   string   `json:"tld"`
	HTTPPort              int      `json:"httpPort"`
	HTTPSPort             int      `json:"httpsPort"`
	MySQLPort             int      `json:"mysqlPort"`
	Parked                []string `json:"parked"`
	StartServicesOnLaunch bool     `json:"startServicesOnLaunch"`
	StopServicesOnQuit    bool     `json:"stopServicesOnQuit"`
	LaunchAtLogin         bool     `json:"launchAtLogin"`
	Home                  string   `json:"home"` // read-only
}

// Backend is everything the GUI and CLI can ask the core to do.
// All methods are safe to call concurrently.
type Backend interface {
	Overview() (Overview, error)

	StartAll() error
	StopAll() error
	RestartAll() error
	StartService(name string) error
	StopService(name string) error
	RestartService(name string) error

	ListSites() ([]Site, error)
	Park(dir string) error
	Unpark(dir string) error
	Link(name, path string) error
	Unlink(name string) error
	SetSitePHP(site, version string) error // version "" = follow default
	SetSiteSecure(site string, secure bool) error
	NewProject(req NewProjectRequest, progress ProgressFunc) (Site, error)

	ListPHP() ([]PHPVersion, error) // installed + available
	InstallPHP(version string, progress ProgressFunc) error
	RemovePHP(version string) error
	SetDefaultPHP(version string) error
	GetPHPSettings(version string) (PHPSettings, error)
	SavePHPSettings(s PHPSettings) error
	PHPIniPath(version string) string

	MySQLInfo() (MySQLInfo, error)
	ListDatabases() ([]Database, error)
	CreateDatabase(name string) error
	DropDatabase(name string) error
	ImportSQL(database, file string) error
	ExportDatabase(database, file string) error
	SetMySQLPassword(password string) error

	LogNames() []string // "apache-error", "apache-access", "mysql", "php-<minor>"
	ReadLog(name string, lines int) (string, error)

	GetSettings() (Settings, error)
	SaveSettings(s Settings) error

	TrustCA() error
}
