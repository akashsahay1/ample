package projects

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"apnoro/internal/api"
)

const laravel11Env = "APP_NAME=Laravel\nAPP_ENV=local\nAPP_URL=http://localhost\n\nDB_CONNECTION=sqlite\n# DB_HOST=127.0.0.1\n# DB_PORT=3306\n# DB_DATABASE=laravel\n# DB_USERNAME=root\n# DB_PASSWORD=\n\nSESSION_DRIVER=database\n"

func TestUpdateEnvLaravel11(t *testing.T) {
	r := Request{Name: "blog", DB: "blog", DBPort: 3307, DBPassword: "p@ss word#1"}
	got := UpdateEnv(laravel11Env, LaravelEnvValues(r))
	want := "APP_NAME=Laravel\nAPP_ENV=local\nAPP_URL=http://blog.test\n\nDB_CONNECTION=mysql\nDB_HOST=127.0.0.1\nDB_PORT=3307\nDB_DATABASE=blog\nDB_USERNAME=root\nDB_PASSWORD=\"p@ss word#1\"\n\nSESSION_DRIVER=database\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	// idempotent
	if again := UpdateEnv(got, LaravelEnvValues(r)); again != got {
		t.Fatalf("not idempotent:\n%s", again)
	}
}

func TestUpdateEnvVariants(t *testing.T) {
	// Laravel 10 style (active DB lines), CRLF, secure, no DB -> only APP_URL
	in := "APP_URL=http://localhost\r\nDB_CONNECTION=mysql\r\nDB_DATABASE=laravel\r\n"
	got := UpdateEnv(in, LaravelEnvValues(Request{Name: "shop", Secure: true, TLD: "localhost"}))
	if got != "APP_URL=https://shop.localhost\r\nDB_CONNECTION=mysql\r\nDB_DATABASE=laravel\r\n" {
		t.Fatalf("got %q", got)
	}
	// no DB leaves sqlite alone
	got = UpdateEnv(laravel11Env, LaravelEnvValues(Request{Name: "x"}))
	if !strings.Contains(got, "DB_CONNECTION=sqlite\n# DB_HOST") {
		t.Fatalf("sqlite changed: %s", got)
	}
	// missing keys appended; empty password stays empty; active wins over commented
	got = UpdateEnv("#DB_PORT=1\nDB_PORT=2\n", [][2]string{{"DB_PORT", "3306"}, {"DB_PASSWORD", ""}})
	if got != "#DB_PORT=1\nDB_PORT=3306\nDB_PASSWORD=\n" {
		t.Fatalf("got %q", got)
	}
	if q := quoteEnv(`a"b$c\`); q != `"a\"b\$c\\"` {
		t.Fatalf("quote %s", q)
	}
}

const wpSample = `<?php
/** The name of the database for WordPress */
define( 'DB_NAME', 'database_name_here' );
define( 'DB_USER', 'username_here' );
define( 'DB_PASSWORD', 'password_here' );
define( 'DB_HOST', 'localhost' );
define( 'DB_CHARSET', 'utf8mb4' );
define( 'AUTH_KEY',         'put your unique phrase here' );
define( 'SECURE_AUTH_KEY',  'put your unique phrase here' );
define( 'LOGGED_IN_KEY',    'put your unique phrase here' );
define( 'NONCE_KEY',        'put your unique phrase here' );
define( 'AUTH_SALT',        'put your unique phrase here' );
define( 'SECURE_AUTH_SALT', 'put your unique phrase here' );
define( 'LOGGED_IN_SALT',   'put your unique phrase here' );
define( 'NONCE_SALT',       'put your unique phrase here' );
$table_prefix = 'wp_';
`

func TestWPConfig(t *testing.T) {
	out, err := WPConfig(wpSample, "blog", "root", `it's\x`, "127.0.0.1:3306")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"define( 'DB_NAME', 'blog' );", "define( 'DB_USER', 'root' );",
		`define( 'DB_PASSWORD', 'it\'s\\x' );`, "define( 'DB_HOST', '127.0.0.1:3306' );",
		"define( 'DB_CHARSET', 'utf8mb4' );", "$table_prefix = 'wp_';",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(out, "put your unique phrase here") {
		t.Error("salts not replaced")
	}
	re := regexp.MustCompile(`define\( '([A-Z_]+_(KEY|SALT))', '([^'\\]{64})' \);`)
	m := re.FindAllStringSubmatch(out, -1)
	if len(m) != 8 {
		t.Fatalf("salts: %d", len(m))
	}
	seen := map[string]bool{}
	for _, x := range m {
		if seen[x[3]] {
			t.Error("duplicate salt")
		}
		seen[x[3]] = true
	}
	if _, err := WPConfig("<?php", "a", "b", "c", "d"); err == nil {
		t.Error("expected error for sample without defines")
	}
}

func TestCreateBlankAndValidation(t *testing.T) {
	dir := t.TempDir()
	p, err := Create(context.Background(), Request{Kind: api.KindBlank, Name: "hello", Dir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "index.php"))
	if err != nil || !strings.Contains(string(b), "hello.test") || !strings.Contains(string(b), "PHP_VERSION") {
		t.Fatalf("index.php: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p, "README.md")); err != nil {
		t.Fatal(err)
	}
	// non-empty target refused
	if _, err := Create(context.Background(), Request{Kind: api.KindBlank, Name: "hello", Dir: dir}, nil); err == nil {
		t.Fatal("non-empty target accepted")
	}
	// empty existing dir is fine
	os.Mkdir(filepath.Join(dir, "empty"), 0o755)
	if _, err := Create(context.Background(), Request{Kind: api.KindBlank, Name: "empty", Dir: dir}, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"", "Bad", "a b", "../x", "-x"} {
		if _, err := Create(context.Background(), Request{Kind: api.KindBlank, Name: n, Dir: dir}, nil); err == nil {
			t.Errorf("name %q accepted", n)
		}
	}
	if _, err := Create(context.Background(), Request{Kind: "rails", Name: "r", Dir: dir}, nil); err == nil {
		t.Error("unknown kind accepted")
	}
	if _, err := Create(context.Background(), Request{Kind: api.KindLaravel, Name: "l", Dir: dir, PHP: filepath.Join(dir, "nophp.exe")}, nil); err == nil {
		t.Error("laravel without php accepted")
	}
}

// Live network test: APNORO_LIVE_TESTS=1 go test ./internal/projects -run Live
func TestLiveWordPress(t *testing.T) {
	if os.Getenv("APNORO_LIVE_TESTS") != "1" {
		t.Skip("set APNORO_LIVE_TESTS=1")
	}
	dir := t.TempDir()
	last := 0.0
	p, err := Create(context.Background(), Request{Kind: api.KindWordPress, Name: "wp", Dir: dir, DB: "wp", DBPort: 3306}, func(msg string, pct float64) {
		if pct-last >= 20 || pct == 100 {
			t.Logf("%.0f%% %s", pct, msg)
			last = pct
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"wp-load.php", "wp-config.php", "wp-admin", "wp-includes"} {
		if _, err := os.Stat(filepath.Join(p, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(p, "wordpress")); err == nil {
		t.Error("top dir not stripped")
	}
	b, _ := os.ReadFile(filepath.Join(p, "wp-config.php"))
	if !strings.Contains(string(b), "'127.0.0.1:3306'") || strings.Contains(string(b), "put your unique phrase here") {
		t.Error("wp-config not generated correctly")
	}
}

// Live Laravel test: needs APNORO_TEST_PHP (php.exe) and APNORO_TEST_COMPOSER (composer.phar).
func TestLiveLaravel(t *testing.T) {
	php, composer := os.Getenv("APNORO_TEST_PHP"), os.Getenv("APNORO_TEST_COMPOSER")
	if php == "" || composer == "" {
		t.Skip("set APNORO_TEST_PHP and APNORO_TEST_COMPOSER")
	}
	dir := t.TempDir()
	p, err := Create(context.Background(), Request{Kind: api.KindLaravel, Name: "lara", Dir: dir, PHP: php, ComposerPhar: composer}, func(msg string, pct float64) { t.Log(msg) })
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(p, ".env"))
	if !strings.Contains(string(b), "APP_URL=http://lara.test") {
		t.Error("APP_URL not set")
	}
}
