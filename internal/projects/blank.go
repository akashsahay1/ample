package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const blankIndex = `<?php
if (isset($_GET['phpinfo'])) { phpinfo(); exit; }
$name = '{{DOMAIN}}';
?><!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title><?= htmlspecialchars($name) ?></title>
<style>
  :root { color-scheme: light dark; }
  body { margin: 0; min-height: 100vh; display: grid; place-items: center;
         font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
         background: #f5f6f8; color: #1d2330; }
  @media (prefers-color-scheme: dark) { body { background: #14171d; color: #e6e9ef; } .card { background: #1d2129 !important; } }
  .card { background: #fff; padding: 40px 48px; border-radius: 16px;
          box-shadow: 0 10px 30px rgba(0,0,0,.08); text-align: center; max-width: 520px; }
  h1 { margin: 0 0 8px; font-size: 28px; }
  p { margin: 6px 0; opacity: .8; }
  code { font-size: 13px; }
  a { color: #4f6bed; }
</style>
</head>
<body>
  <div class="card">
    <h1>Hello from <?= htmlspecialchars($name) ?></h1>
    <p>Running PHP <strong><?= PHP_VERSION ?></strong> on Apnoro.</p>
    <p>Edit <code><?= htmlspecialchars(__FILE__) ?></code> to get started.</p>
    <p><a href="?phpinfo=1">View phpinfo()</a></p>
  </div>
</body>
</html>
`

const blankReadme = `# {{NAME}}

A blank PHP project served by Apnoro at {{URL}}

- ` + "`index.php`" + ` is the entry point.
- Pin a PHP version for the command line by writing e.g. ` + "`8.3`" + ` to a ` + "`.apnoro-php`" + ` file,
  or isolate the site's PHP version from the Apnoro app / ` + "`apnoro isolate`" + `.
`

func createBlank(r Request, target string) error {
	if err := os.MkdirAll(target, 0o755); err != nil {
		return fmt.Errorf("projects: blank: %w", err)
	}
	domain := r.Name + "." + r.TLD
	idx := strings.ReplaceAll(blankIndex, "{{DOMAIN}}", domain)
	readme := strings.NewReplacer("{{NAME}}", r.Name, "{{URL}}", appURL(r)).Replace(blankReadme)
	if err := os.WriteFile(filepath.Join(target, "index.php"), []byte(idx), 0o644); err != nil {
		return fmt.Errorf("projects: blank: %w", err)
	}
	if err := os.WriteFile(filepath.Join(target, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("projects: blank: %w", err)
	}
	return nil
}
