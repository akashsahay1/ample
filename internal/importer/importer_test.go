package importer

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"ampls/internal/api"
	"ampls/internal/config"
	"ampls/internal/external"
	"ampls/internal/paths"
)

func TestMain(m *testing.M) {
	paths.SetHome(filepath.Join(os.TempDir(), "ampls-importer-test-home"))
	os.Exit(m.Run())
}

func TestFilterLine(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		mariadb bool
		want    string // "" with drop=true means dropped
		drop    bool
	}{
		{"sandbox", "/*M!999999\\- enable the sandbox mode */ \n", true, "", true},
		{"maria versioned", "/*!100616 SET @OLD_NOTE_VERBOSITY=@@NOTE_VERBOSITY, NOTE_VERBOSITY=0 */;\n", true, "", true},
		{"mysql versioned kept", "/*!40101 SET NAMES utf8mb4 */;\n", true, "/*!40101 SET NAMES utf8mb4 */;\n", false},
		{"uca1400 table", ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci;\n", true,
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;\n", false},
		{"uca1400 as_cs column", "  `n` varchar(10) COLLATE utf8mb4_uca1400_as_cs NOT NULL,\n", true,
			"  `n` varchar(10) COLLATE utf8mb4_0900_as_cs NOT NULL,\n", false},
		{"utf8mb3 uca1400", "  `n` text CHARACTER SET utf8mb3 COLLATE utf8mb3_uca1400_ai_ci,\n", true,
			"  `n` text CHARACTER SET utf8mb3 COLLATE utf8mb3_general_ci,\n", false},
		{"nopad", "COLLATE=utf8mb4_unicode_nopad_ci;\n", true, "COLLATE=utf8mb4_unicode_ci;\n", false},
		{"nopad bin", "COLLATE=utf8mb4_nopad_bin;\n", true, "COLLATE=utf8mb4_bin;\n", false},
		{"aria options", ") ENGINE=Aria DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_general_ci PAGE_CHECKSUM=1 TRANSACTIONAL=1 ROW_FORMAT=PAGE;\n", true,
			") ENGINE=InnoDB DEFAULT CHARSET=utf8mb3 COLLATE=utf8mb3_general_ci;\n", false},
		{"json check name", "  CONSTRAINT `options` CHECK (json_valid(`options`))\n", true, "  CHECK (json_valid(`options`))\n", false},
		{"definer view", "/*!50013 DEFINER=`pma`@`%` SQL SECURITY DEFINER */\n", false,
			"/*!50013 DEFINER=CURRENT_USER SQL SECURITY DEFINER */\n", false},
		{"definer routine", "CREATE DEFINER=`root`@`localhost` PROCEDURE `p`()\n", true,
			"CREATE DEFINER=CURRENT_USER PROCEDURE `p`()\n", false},
		{"insert untouched", "INSERT INTO `t` VALUES (1,'utf8mb4_uca1400_ai_ci DEFINER=`a`@`b` ENGINE=Aria');\n", true,
			"INSERT INTO `t` VALUES (1,'utf8mb4_uca1400_ai_ci DEFINER=`a`@`b` ENGINE=Aria');\n", false},
		{"mysql source no maria rewrites", ") ENGINE=Aria;\n", false, ") ENGINE=Aria;\n", false},
		{"crlf", "/*M!999999\\- enable the sandbox mode */\r\n", true, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, keep := FilterLine([]byte(c.in), c.mariadb)
			if keep == c.drop {
				t.Fatalf("keep = %v, want %v", keep, !c.drop)
			}
			if keep && string(out) != c.want {
				t.Errorf("got  %q\nwant %q", out, c.want)
			}
		})
	}
}

func TestFilterDumpStream(t *testing.T) {
	big := "INSERT INTO `t` VALUES ('" + strings.Repeat("x", 3<<20) + "');\n" // longer than the reader buffer
	in := "/*M!999999\\- enable the sandbox mode */ \n" +
		"-- MariaDB dump 10.19\n" +
		"CREATE TABLE `t` (\n  `a` longtext\n) ENGINE=Aria DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_uca1400_ai_ci PAGE_CHECKSUM=1;\n" +
		big +
		"-- Dump completed" // no trailing newline
	var out bytes.Buffer
	if err := FilterDump(strings.NewReader(in), &out, true); err != nil {
		t.Fatal(err)
	}
	want := "-- MariaDB dump 10.19\n" +
		"CREATE TABLE `t` (\n  `a` longtext\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;\n" +
		big + "-- Dump completed"
	if out.String() != want {
		t.Errorf("stream mismatch: got %d bytes, want %d; head %q", out.Len(), len(want), out.String()[:min(200, out.Len())])
	}
}

func TestMapCollation(t *testing.T) {
	for in, want := range map[string]string{
		"utf8mb4_uca1400_ai_ci": "utf8mb4_0900_ai_ci",
		"utf8mb4_general_ci":    "utf8mb4_general_ci",
		"latin1_swedish_ci":     "latin1_swedish_ci",
		"utf8mb3_uca1400_as_ci": "utf8mb3_general_ci",
	} {
		if got := MapCollation(in); got != want {
			t.Errorf("MapCollation(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDumpArgs(t *testing.T) {
	has := func(args []string, a string) bool {
		for _, x := range args {
			if x == a {
				return true
			}
		}
		return false
	}
	a := DumpArgs("shop", false, true, nil)
	if !has(a, "--column-statistics=0") || has(a, "--set-gtid-purged=OFF") || a[len(a)-1] != "shop" {
		t.Errorf("mysql tool vs mariadb: %v", a)
	}
	a = DumpArgs("shop", false, false, nil)
	if has(a, "--column-statistics=0") || !has(a, "--set-gtid-purged=OFF") {
		t.Errorf("mysql tool vs mysql: %v", a)
	}
	a = DumpArgs("shop", true, true, nil)
	if has(a, "--column-statistics=0") || has(a, "--set-gtid-purged=OFF") {
		t.Errorf("mariadb tool: %v", a)
	}
	a = DumpArgs("shop", false, true, func(f string) bool { return f != "column-statistics" })
	if has(a, "--column-statistics=0") {
		t.Errorf("unsupported flag passed: %v", a)
	}
	for _, need := range []string{"--single-transaction", "--routines", "--triggers", "--events", "--default-character-set=utf8mb4", "--skip-add-locks"} {
		if !has(a, need) {
			t.Errorf("missing %s", need)
		}
	}
}

func xamppPlan(t *testing.T, existing []api.Site, installed []string) (api.ImportPlan, string) {
	t.Helper()
	root, _ := filepath.Abs(filepath.Join("..", "external", "testdata", "xampp"))
	list, parked, notes, err := external.SourceSites(api.EnvXAMPP, root)
	if err != nil {
		t.Fatal(err)
	}
	plan := api.ImportPlan{Kind: api.EnvXAMPP, Source: root, Sites: list, ParkedDirs: parked, Notes: notes}
	annotate(&plan, existing, installed)
	return plan, root
}

func TestAnnotate(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", "external", "testdata", "xampp"))
	existing := []api.Site{
		{Name: "blog", Domain: "blog.test", Path: `C:\elsewhere\blog`},
		{Name: "old-shop", Domain: "old-shop.test", Path: filepath.Join(root, "htdocs", "My Shop")},
	}
	plan := api.ImportPlan{Sites: []api.ImportSite{
		{Name: "blog", Path: filepath.Join(root, "htdocs", "blog")},
		{Name: "shop", Path: filepath.Join(root, "htdocs", "My Shop")},
		{Name: "", Path: `C:\x\___`},
		{Name: "api", Path: `C:\a`, PHP: "8.1"},
		{Name: "api", Path: `C:\b`, PHP: "8.3"},
		{Name: "gone", Path: `C:\c`, Conflict: "folder not found"},
	}}
	annotate(&plan, existing, []string{"8.3", "8.4"})
	wantPrefix := []string{"name blog is taken", "already served by AMPLS as old-shop.test", "\"\" is not a valid", "", "another imported site", "folder not found"}
	for i, w := range wantPrefix {
		c := plan.Sites[i].Conflict
		if (w == "" && c != "") || (w != "" && !strings.HasPrefix(c, w)) {
			t.Errorf("[%d] conflict = %q, want prefix %q", i, c, w)
		}
	}
	if !reflect.DeepEqual(plan.MissingPHP, []string{"8.1"}) {
		t.Errorf("MissingPHP = %v", plan.MissingPHP)
	}
}

func TestPlanSitesLinkAndPark(t *testing.T) {
	plan, root := xamppPlan(t, nil, []string{"8.4"})
	htdocs := filepath.Join(root, "htdocs")
	shop := filepath.Join(htdocs, "My Shop")

	// Link individually, keep https.
	var res Result
	planSites(&res, plan, api.ImportRequest{Kind: "xampp", Sites: []string{shop, `C:\not\in\plan`}, KeepSecure: true}, nil)
	if !reflect.DeepEqual(res.Links, []config.Link{{Name: "shop", Path: shop}}) {
		t.Errorf("links = %+v", res.Links)
	}
	if !reflect.DeepEqual(res.Secure, []string{"shop"}) {
		t.Errorf("secure = %v", res.Secure)
	}
	if len(res.DocRoots) != 0 { // public/ is what AMPLS detects anyway
		t.Errorf("docroots = %v", res.DocRoots)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], `C:\not\in\plan`) && !strings.Contains(res.Notes[0], strings.ToLower(`C:\not\in\plan`)) {
		t.Errorf("notes = %v", res.Notes)
	}

	// Park htdocs: no links; the vhost site is served under its folder name.
	res = Result{}
	planSites(&res, plan, api.ImportRequest{Kind: "xampp", ParkDirs: []string{strings.ToUpper(htdocs)}, Sites: []string{shop}, KeepSecure: true}, nil)
	if len(res.Links) != 0 || !reflect.DeepEqual(res.Park, []string{htdocs}) {
		t.Errorf("park: links=%+v park=%v", res.Links, res.Park)
	}
	if !reflect.DeepEqual(res.Secure, []string{"my-shop"}) {
		t.Errorf("secure = %v", res.Secure)
	}
}

func TestPlanSitesPHP(t *testing.T) {
	plan := api.ImportPlan{Kind: "herd", Sites: []api.ImportSite{
		{Name: "a", Path: `C:\p\a`, PHP: "8.4", Source: "link"},
		{Name: "b", Path: `C:\p\b`, PHP: "8.1", Source: "link"},
		{Name: "c", Path: `C:\p\c`, DocRoot: `C:\p\c\htdocs`, Source: "link"},
	}}
	req := api.ImportRequest{Sites: []string{`C:\p\a`, `C:\p\b`, `C:\p\c`}}
	var res Result
	planSites(&res, plan, req, []string{"8.4"})
	if !reflect.DeepEqual(res.PHP, map[string]string{"a": "8.4"}) || len(res.InstallPHP) != 0 {
		t.Errorf("no install: php=%v install=%v", res.PHP, res.InstallPHP)
	}
	if res.DocRoots["c"] != "htdocs" {
		t.Errorf("docroots = %v", res.DocRoots)
	}
	req.InstallPHP = true
	res = Result{}
	planSites(&res, plan, req, []string{"8.4"})
	if !reflect.DeepEqual(res.PHP, map[string]string{"a": "8.4", "b": "8.1"}) || !reflect.DeepEqual(res.InstallPHP, []string{"8.1"}) {
		t.Errorf("install: php=%v install=%v", res.PHP, res.InstallPHP)
	}
}

func TestClientCnfEscapes(t *testing.T) {
	f, err := clientCnf(&server{Host: "127.0.0.1", Port: 3307, User: "root", Password: `p"a\ss`})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f)
	b, _ := os.ReadFile(f)
	if !strings.Contains(string(b), `password="p\"a\\ss"`) || !strings.Contains(string(b), "port=3307") {
		t.Errorf("cnf = %s", b)
	}
}
