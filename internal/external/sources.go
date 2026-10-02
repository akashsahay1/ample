package external

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"apnoro/internal/api"
	"apnoro/internal/php"
	"apnoro/internal/sites"
)

// ---------- XAMPP ----------

// XAMPPInfo is what Apnoro reads from an XAMPP installation.
type XAMPPInfo struct {
	Root           string
	Version        string // "8.2.12"
	PHPVersion     string // "8.2.12"
	MariaDBVersion string // "10.4.32"
	MySQLPort      int    // from mysql\bin\my.ini (default 3306)
	HTTPPort       int    // Listen in apache\conf\httpd.conf (default 80)
	Htdocs         string
	MyIni          string // mysql\bin\my.ini ("" if missing)
}

var (
	xamppVerRe   = regexp.MustCompile(`(?i)XAMPP\s+(?:for\s+Windows\s+)?(?:Version\s+)?(\d+\.\d+\.\d+(?:-\d+)?)`)
	xamppPHPRe   = regexp.MustCompile(`(?im)^\s*\+\s*PHP\s+(\d+\.\d+\.\d+)`)
	xamppMariaRe = regexp.MustCompile(`(?im)^\s*\+\s*MariaDB\s+(\d+\.\d+\.\d+)`)
	listenRe     = regexp.MustCompile(`(?im)^\s*Listen\s+(?:[\d.]+:|\[[^\]]*\]:)?(\d+)\s*$`)
)

// ReadXAMPP inspects an XAMPP root (never starts anything except, when the
// readme does not tell, `php -r` with a 5s timeout to learn the PHP version).
func ReadXAMPP(root string) XAMPPInfo {
	x := XAMPPInfo{Root: root, MySQLPort: 3306, HTTPPort: 80, Htdocs: filepath.Join(root, "htdocs")}
	for _, name := range []string{"readme_en.txt", "readme_de.txt"} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		parseXAMPPReadme(string(b), &x)
		break
	}
	if x.Version == "" {
		if b, err := os.ReadFile(filepath.Join(root, "properties.ini")); err == nil {
			if m := xamppVerRe.FindStringSubmatch(string(b)); m != nil {
				x.Version = m[1]
			}
		}
	}
	if x.PHPVersion == "" {
		x.PHPVersion = phpVersionOf(filepath.Join(root, "php", "php.exe"))
	}
	ini := filepath.Join(root, "mysql", "bin", "my.ini")
	if isFile(ini) {
		x.MyIni = ini
		if b, err := os.ReadFile(ini); err == nil {
			if p := IniInt(string(b), "mysqld", "port"); p > 0 {
				x.MySQLPort = p
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "apache", "conf", "httpd.conf")); err == nil {
		if m := listenRe.FindStringSubmatch(string(b)); m != nil {
			x.HTTPPort, _ = strconv.Atoi(m[1])
		}
	}
	return x
}

func parseXAMPPReadme(s string, x *XAMPPInfo) {
	if m := xamppVerRe.FindStringSubmatch(s); m != nil {
		x.Version = m[1]
	}
	if m := xamppPHPRe.FindStringSubmatch(s); m != nil {
		x.PHPVersion = m[1]
	}
	if m := xamppMariaRe.FindStringSubmatch(s); m != nil {
		x.MariaDBVersion = m[1]
	}
}

// IniInt returns key's integer value inside [section] of an ini/my.cnf text.
func IniInt(content, section, key string) int {
	v := IniValue(content, section, key)
	n, _ := strconv.Atoi(v)
	return n
}

// IniValue returns key's value inside [section] ("" if absent). Quotes are removed.
func IniValue(content, section, key string) string {
	cur := ""
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		if line[0] == '[' && strings.HasSuffix(line, "]") {
			cur = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		if cur != strings.ToLower(section) {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(k)), "-", "_")
		if k == strings.ReplaceAll(strings.ToLower(key), "-", "_") {
			v = strings.TrimSpace(v)
			if i := strings.IndexAny(v, "#;"); i >= 0 && !strings.HasPrefix(v, `"`) {
				v = strings.TrimSpace(v[:i])
			}
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

// ---------- Laragon / WAMP ----------

type stackInfo struct {
	root    string
	php     []string // full versions found under bin\php
	hasData bool
	tld     string
}

func (s stackInfo) newestPHP() string {
	if len(s.php) == 0 {
		return ""
	}
	return s.php[0]
}

var verInName = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

func versionsIn(dir string) []string {
	var out []string
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if m := verInName.FindStringSubmatch(e.Name()); e.IsDir() && m != nil {
			out = append(out, m[1])
		}
	}
	sort.Slice(out, func(i, j int) bool { return php.Compare(out[i], out[j]) > 0 })
	return out
}

// ReadLaragon inspects a Laragon root.
func ReadLaragon(root string) stackInfo {
	s := stackInfo{root: root, tld: "test", php: versionsIn(filepath.Join(root, "bin", "php"))}
	if b, err := os.ReadFile(filepath.Join(root, "usr", "laragon.ini")); err == nil {
		if m := regexp.MustCompile(`\{name\}\.([a-z0-9-]+)`).FindStringSubmatch(string(b)); m != nil {
			s.tld = m[1]
		}
	}
	ents, _ := os.ReadDir(filepath.Join(root, "data"))
	for _, e := range ents {
		n := strings.ToLower(e.Name())
		if e.IsDir() && (strings.HasPrefix(n, "mysql") || strings.HasPrefix(n, "mariadb")) &&
			len(DataDirDatabases(filepath.Join(root, "data", e.Name()))) > 0 {
			s.hasData = true
		}
	}
	return s
}

// ReadWAMP inspects a WampServer root.
func ReadWAMP(root string) stackInfo {
	s := stackInfo{root: root, php: versionsIn(filepath.Join(root, "bin", "php"))}
	for _, eng := range []string{"mysql", "mariadb"} {
		ents, _ := os.ReadDir(filepath.Join(root, "bin", eng))
		for _, e := range ents {
			if e.IsDir() && len(DataDirDatabases(filepath.Join(root, "bin", eng, e.Name(), "data"))) > 0 {
				s.hasData = true
			}
		}
	}
	return s
}

// ---------- Sites ----------

// htdocs/www entries shipped by the stacks themselves.
var stackFolders = map[string]bool{
	"dashboard": true, "img": true, "webalizer": true, "xampp": true, "applets": true,
	"forbidden": true, "restricted": true, "wamplangues": true, "wampthemes": true,
	"cgi-bin": true, ".well-known": true,
}

// SourceSites lists the sites an environment serves, without Apnoro-side
// conflict checks: parked lists the directories that could be parked instead
// of linking their folders one by one.
func SourceSites(kind, root string) (list []api.ImportSite, parked []string, notes []string, err error) {
	switch kind {
	case api.EnvXAMPP:
		x := ReadXAMPP(root)
		list = folderSites(x.Htdocs, "htdocs", func(f string) string { return "localhost/" + f })
		vars := map[string]string{"SRVROOT": filepath.ToSlash(filepath.Join(root, "apache"))}
		if b, err := os.ReadFile(filepath.Join(root, "apache", "conf", "extra", "httpd-vhosts.conf")); err == nil {
			list = mergeVHosts(list, ParseVHosts(string(b), vars, x.Htdocs))
		}
		if isDir(x.Htdocs) {
			parked = []string{x.Htdocs}
		}
		if x.PHPVersion != "" {
			notes = append(notes, fmt.Sprintf("XAMPP uses PHP %s; imported sites will use the Apnoro default PHP unless you pin them.", x.PHPVersion))
		}
	case api.EnvHerd:
		h, err := ReadHerd(root)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("herd: read config: %w", err)
		}
		for _, dir := range h.Paths {
			if !isDir(dir) {
				notes = append(notes, "Herd parked folder "+dir+" does not exist; skipped.")
				continue
			}
			parked = append(parked, dir)
			for _, s := range folderSites(dir, "parked", func(f string) string { return sites.Slug(f) + "." + h.TLD }) {
				key := strings.ToLower(filepath.Base(s.Path))
				s.PHP = h.Isolated[key]
				s.Secure = h.Secured[key]
				list = append(list, s)
			}
		}
		for _, l := range h.Links {
			key := strings.ToLower(l.Name)
			s := api.ImportSite{
				Name: sites.Slug(l.Name), Domain: key + "." + h.TLD, Path: l.Path,
				PHP: h.Isolated[key], Secure: h.Secured[key], Source: "link",
			}
			if isDir(l.Path) {
				s.DocRoot = sites.DocRoot(l.Path)
			} else {
				s.Conflict = "folder not found"
			}
			list = append(list, s)
		}
		if len(h.PHP) > 0 {
			var vs []string
			for _, p := range h.PHP {
				vs = append(vs, p.Full)
			}
			notes = append(notes, fmt.Sprintf("Herd bundles PHP %s (global %s).", strings.Join(vs, ", "), h.ActivePHP))
		}
		if len(h.Services) > 0 {
			notes = append(notes, "Herd Pro services found: "+strings.Join(h.Services, ", ")+". To copy Herd's MySQL databases, provide its connection (default root, no password, port 3306) while Herd is running.")
		}
	case api.EnvLaragon:
		l := ReadLaragon(root)
		www := filepath.Join(root, "www")
		list = folderSites(www, "parked", func(f string) string { return sites.Slug(f) + "." + l.tld })
		if isDir(www) {
			parked = []string{www}
		}
		if len(l.php) > 0 {
			notes = append(notes, "Laragon PHP versions: "+strings.Join(l.php, ", ")+"; imported sites will use the Apnoro default PHP unless you pin them.")
		}
	case api.EnvWAMP:
		w := ReadWAMP(root)
		www := filepath.Join(root, "www")
		list = folderSites(www, "htdocs", func(f string) string { return "localhost/" + f })
		vars := map[string]string{"INSTALL_DIR": filepath.ToSlash(root)}
		ents, _ := os.ReadDir(filepath.Join(root, "bin", "apache"))
		for _, e := range ents {
			conf := filepath.Join(root, "bin", "apache", e.Name(), "conf", "extra", "httpd-vhosts.conf")
			if b, err := os.ReadFile(conf); err == nil {
				vars["SRVROOT"] = filepath.ToSlash(filepath.Join(root, "bin", "apache", e.Name()))
				list = mergeVHosts(list, ParseVHosts(string(b), vars, www))
			}
		}
		if isDir(www) {
			parked = []string{www}
		}
		if len(w.php) > 0 {
			notes = append(notes, "WampServer PHP versions: "+strings.Join(w.php, ", ")+"; imported sites will use the Apnoro default PHP unless you pin them.")
		}
	case api.EnvMySQL:
		return nil, nil, nil, nil
	default:
		return nil, nil, nil, fmt.Errorf("unknown environment %q", kind)
	}
	return list, parked, notes, nil
}

// folderSites lists the project folders directly inside dir.
func folderSites(dir, source string, domain func(folder string) string) []api.ImportSite {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []api.ImportSite
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || stackFolders[strings.ToLower(name)] {
			continue
		}
		full := filepath.Join(dir, name)
		if !e.IsDir() {
			// Follow directory symlinks/junctions.
			if e.Type()&fs.ModeSymlink == 0 && e.Type()&fs.ModeIrregular == 0 || !isDir(full) {
				continue
			}
		}
		out = append(out, api.ImportSite{
			Name: sites.Slug(name), Domain: domain(name), Path: full,
			DocRoot: sites.DocRoot(full), Source: source,
		})
	}
	return out
}

// VHost is one <VirtualHost> block.
type VHost struct {
	ServerName   string
	DocumentRoot string // OS path
	SSL          bool
}

var varRe = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

// ParseVHosts extracts named virtual hosts from an Apache vhosts file.
// Commented lines, the stock example (dummy-host) blocks, localhost and blocks
// serving defaultRoot itself (the stack's own localhost) are skipped.
// vars expands ${NAME}; `Define NAME value` lines add to it.
func ParseVHosts(content string, vars map[string]string, defaultRoot string) []VHost {
	v := map[string]string{}
	for k, val := range vars {
		v[k] = val
	}
	expand := func(s string) string {
		return varRe.ReplaceAllStringFunc(s, func(m string) string {
			if r, ok := v[m[2:len(m)-1]]; ok {
				return r
			}
			return m
		})
	}
	var out []VHost
	var cur *VHost
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r", ""), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := splitDirective(line)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(fields[0])
		switch {
		case key == "define" && len(fields) >= 3:
			v[fields[1]] = expand(fields[2])
		case strings.HasPrefix(key, "<virtualhost"):
			cur = &VHost{SSL: strings.Contains(line, ":443")}
		case key == "</virtualhost>":
			if cur != nil {
				out = append(out, *cur)
			}
			cur = nil
		case cur == nil:
		case key == "servername" && len(fields) >= 2:
			name := strings.ToLower(fields[1])
			if i := strings.LastIndex(name, ":"); i > 0 {
				name = name[:i]
			}
			cur.ServerName = name
		case key == "documentroot" && len(fields) >= 2:
			cur.DocumentRoot = filepath.Clean(filepath.FromSlash(expand(fields[1])))
		case key == "sslengine" && len(fields) >= 2 && strings.EqualFold(fields[1], "on"):
			cur.SSL = true
		}
	}
	// Filter and merge :80/:443 twins.
	var res []VHost
	idx := map[string]int{}
	for _, h := range out {
		if h.ServerName == "" || h.DocumentRoot == "" || h.ServerName == "localhost" ||
			strings.Contains(h.ServerName, "dummy-host") || strings.HasPrefix(h.ServerName, "127.") {
			continue
		}
		if defaultRoot != "" && normPath(h.DocumentRoot) == normPath(defaultRoot) {
			continue
		}
		if i, ok := idx[h.ServerName]; ok {
			res[i].SSL = res[i].SSL || h.SSL
			continue
		}
		idx[h.ServerName] = len(res)
		res = append(res, h)
	}
	return res
}

// splitDirective splits an Apache directive line, honouring double quotes.
func splitDirective(line string) []string {
	var out []string
	var b strings.Builder
	inQ := false
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == '"':
			inQ = !inQ
		case (r == ' ' || r == '\t') && !inQ:
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
}

// VHostSite converts a vhost to an import site: name is the slug of the
// ServerName's first label (after a leading "www."); a docroot ending in
// public/web makes the parent the project folder.
func VHostSite(h VHost) api.ImportSite {
	label := strings.TrimPrefix(h.ServerName, "www.")
	if i := strings.Index(label, "."); i > 0 {
		label = label[:i]
	}
	s := api.ImportSite{Name: sites.Slug(label), Domain: h.ServerName, Path: h.DocumentRoot,
		DocRoot: h.DocumentRoot, Secure: h.SSL, Source: "vhost"}
	if b := strings.ToLower(filepath.Base(h.DocumentRoot)); b == "public" || b == "web" {
		s.Path = filepath.Dir(h.DocumentRoot)
	}
	if !isDir(h.DocumentRoot) {
		s.Conflict = "folder not found"
	}
	return s
}

// mergeVHosts appends vhost sites; a vhost replaces a folder entry with the same project path.
func mergeVHosts(list []api.ImportSite, hosts []VHost) []api.ImportSite {
	for _, h := range hosts {
		s := VHostSite(h)
		replaced := false
		for i := range list {
			if list[i].Source != "vhost" && normPath(list[i].Path) == normPath(s.Path) {
				list[i] = s
				replaced = true
				break
			}
		}
		if !replaced {
			list = append(list, s)
		}
	}
	return list
}

// ---------- MySQL data directories ----------

// SystemDatabases are never imported.
var SystemDatabases = map[string]bool{
	"information_schema": true, "performance_schema": true, "mysql": true, "sys": true,
	"phpmyadmin": true, "test": true,
}

// DataDirDatabases lists user databases in a (stopped) MySQL/MariaDB datadir
// by looking at its sub-directories. Sizes are the sum of the files inside.
func DataDirDatabases(dataDir string) []api.Database {
	ents, err := os.ReadDir(dataDir)
	if err != nil {
		return nil
	}
	var out []api.Database
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "#") || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		name := DecodeDirName(e.Name())
		if SystemDatabases[strings.ToLower(name)] {
			continue
		}
		d := api.Database{Name: name}
		files, _ := os.ReadDir(filepath.Join(dataDir, e.Name()))
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			switch low := strings.ToLower(f.Name()); {
			case strings.HasSuffix(low, ".frm"):
				d.Tables++
			case strings.HasSuffix(low, ".ibd") && !hasFrmTwin(files, f.Name()):
				d.Tables++ // MySQL 8 has no .frm files
			}
			if info, err := f.Info(); err == nil {
				d.SizeBytes += info.Size()
			}
		}
		out = append(out, d)
	}
	return out
}

func hasFrmTwin(files []os.DirEntry, ibd string) bool {
	base := strings.TrimSuffix(strings.TrimSuffix(ibd, ".ibd"), ".IBD")
	for _, f := range files {
		if strings.EqualFold(f.Name(), base+".frm") {
			return true
		}
	}
	return false
}

var encRe = regexp.MustCompile(`@([0-9a-fA-F]{4})`)

// DecodeDirName reverses MySQL's filename encoding ("my@002dapp" -> "my-app").
func DecodeDirName(s string) string {
	return encRe.ReplaceAllStringFunc(s, func(m string) string {
		n, err := strconv.ParseUint(m[1:], 16, 32)
		if err != nil {
			return m
		}
		return string(rune(n))
	})
}
