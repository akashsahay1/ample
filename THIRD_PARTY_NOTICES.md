# Third-party notices

Apnoro itself is MIT-licensed (see [LICENSE](LICENSE)). The installer redistributes the programs below. Each keeps its own license; upstream license texts are installed under `LICENSES\` in the Apnoro data directory.

MySQL and phpMyAdmin are redistributed **unmodified**, as separate programs that Apnoro starts as their own processes and talks to over normal interfaces (SQL over TCP, HTTP). Apnoro does not link against them. Source code for these components is available from their upstream projects.

| Component | Bundled version | License | Upstream |
|---|---|---|---|
| Apache HTTP Server (Apache Lounge Windows build) | 2.4.68 | Apache License 2.0 | https://httpd.apache.org, https://www.apachelounge.com |
| mod_fcgid | 2.3.10 | Apache License 2.0 | https://httpd.apache.org/mod_fcgid/ |
| PHP | 8.5.11 (more versions installable at runtime) | PHP License v3.01 (see note) | https://www.php.net |
| MySQL Community Server | 8.4.11 | GPLv2 with the Universal FOSS Exception | https://dev.mysql.com |
| phpMyAdmin | 5.2.3 | GPLv2 | https://www.phpmyadmin.net |
| Composer | 2.10.3 | MIT | https://getcomposer.org |
| Wails (desktop app framework) | v2 | MIT | https://wails.io |
| Microsoft Visual C++ Redistributable | 14.51 (x64) | Microsoft software license terms (redistributable) | https://aka.ms/vs/17/release/vc_redist.x64.exe |
| curl CA certificate bundle (`cacert.pem`) | 2026-09-29 | MPL 2.0 (Mozilla CA list, via curl.se) | https://curl.se/docs/caextract.html |

## Notes

**PHP license.** PHP releases up to and including 8.5 are distributed under the PHP License v3.01 (with the Zend Engine License v2.0 covering the Zend Engine). The PHP project has replaced these with a simplified three-clause license equivalent to the Modified BSD (BSD-3-Clause) license; per public reporting this takes effect starting with PHP 8.6 (2026), not PHP 8.4. The bundled PHP 8.5.11 therefore remains under PHP License v3.01. PHP versions that Apnoro downloads later from windows.php.net carry whichever license their release ships with. The license text is included in the PHP folder of each release.

**MySQL.** MySQL Community Server is licensed under the GNU General Public License, version 2, with additional permissions in the Universal FOSS Exception, version 1.0 (https://oss.oracle.com/licenses/universal-foss-exception/). "MySQL" is a trademark of Oracle and/or its affiliates; Apnoro is not affiliated with or endorsed by Oracle.

**Go dependencies.** The Apnoro binaries statically include pure-Go libraries (see `go.mod`), each under its own permissive or weak-copyleft license; the desktop UI uses React and other npm packages listed in `frontend/package.json`.

**Trademarks.** Apache, Apache HTTP Server, PHP, MySQL, phpMyAdmin, Composer, Laravel and WordPress are trademarks of their respective owners. Apnoro is an independent project.
