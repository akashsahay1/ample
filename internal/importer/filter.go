package importer

import (
	"bufio"
	"bytes"
	"io"
	"regexp"
)

// FilterDump copies a mysqldump stream from r to w, rewriting the constructs
// that MySQL 8.4 rejects. mariadb enables the MariaDB-specific rewrites; the
// DEFINER rewrite always applies so views/routines/triggers do not reference
// accounts that do not exist in AMPLS.
//
// Data lines (INSERT ...) are passed through untouched so values are never altered.
func FilterDump(r io.Reader, w io.Writer, mariadb bool) error {
	br := bufio.NewReaderSize(r, 1<<20)
	bw := bufio.NewWriterSize(w, 1<<20)
	for {
		line, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			// Very long line (extended INSERT): stream it through unchanged.
			if _, werr := bw.Write(line); werr != nil {
				return werr
			}
			for err == bufio.ErrBufferFull {
				line, err = br.ReadSlice('\n')
				if _, werr := bw.Write(line); werr != nil {
					return werr
				}
			}
			if err != nil && err != io.EOF {
				return err
			}
			if err == io.EOF {
				break
			}
			continue
		}
		if len(line) > 0 {
			out, keep := FilterLine(line, mariadb)
			if keep {
				if _, werr := bw.Write(out); werr != nil {
					return werr
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return bw.Flush()
}

var (
	pInsert       = []byte("INSERT ")
	pReplace      = []byte("REPLACE ")
	pSandbox      = []byte("/*M!")
	definerRe     = regexp.MustCompile("DEFINER=`(?:[^`]|``)*`@`(?:[^`]|``)*`")
	mariaVerRe    = regexp.MustCompile(`^/\*!1\d{5}\s.*\*/;?\s*$`)
	uca4Re        = regexp.MustCompile(`\butf8mb4_uca1400(?:_nopad)?_(ai_ci|as_ci|as_cs|ai_cs)\b`)
	uca3Re        = regexp.MustCompile(`\butf8mb3_uca1400(?:_nopad)?_\w+\b`)
	ucaBareRe     = regexp.MustCompile(`\buca1400(?:_nopad)?_(ai_ci|as_ci|as_cs|ai_cs)\b`)
	nopadRe       = regexp.MustCompile(`\b(utf8mb4|utf8mb3|utf8|latin1)_(unicode|general)_nopad_ci\b`)
	nopadBinRe    = regexp.MustCompile(`\b(utf8mb4|utf8mb3|utf8|latin1)_nopad_bin\b`)
	pageCkRe      = regexp.MustCompile(`\s+PAGE_CHECKSUM=\d`)
	transRe       = regexp.MustCompile(`\s+TRANSACTIONAL=\d`)
	rowFmtPageRe  = regexp.MustCompile(`\s+ROW_FORMAT=PAGE\b`)
	ariaRe        = regexp.MustCompile(`\bENGINE=Aria\b`)
	checkNameRe   = regexp.MustCompile("CONSTRAINT `(?:[^`]|``)*` CHECK \\(")
	uca4Map       = map[string]string{"ai_ci": "utf8mb4_0900_ai_ci", "as_ci": "utf8mb4_0900_as_ci", "as_cs": "utf8mb4_0900_as_cs", "ai_cs": "utf8mb4_0900_as_cs"}
	nopadGenerics = map[string]string{"utf8": "utf8mb3"}
)

// FilterLine rewrites one dump line; keep=false drops it.
func FilterLine(line []byte, mariadb bool) (out []byte, keep bool) {
	if bytes.HasPrefix(line, pInsert) || bytes.HasPrefix(line, pReplace) {
		return line, true
	}
	if mariadb {
		// "/*M!999999\- enable the sandbox mode */" and other MariaDB-only
		// executable comments; MySQL's client chokes on the "\-".
		if bytes.HasPrefix(line, pSandbox) {
			return nil, false
		}
		// Whole-line comments versioned for MariaDB 10+ (/*!100616 ... */).
		if mariaVerRe.Match(bytes.TrimRight(line, "\r\n")) {
			return nil, false
		}
		line = uca4Re.ReplaceAllFunc(line, func(m []byte) []byte {
			return []byte(uca4Map[string(uca4Re.FindSubmatch(m)[1])])
		})
		line = uca3Re.ReplaceAll(line, []byte("utf8mb3_general_ci"))
		line = ucaBareRe.ReplaceAllFunc(line, func(m []byte) []byte {
			return []byte(uca4Map[string(ucaBareRe.FindSubmatch(m)[1])])
		})
		line = nopadRe.ReplaceAllFunc(line, func(m []byte) []byte {
			sm := nopadRe.FindSubmatch(m)
			cs := string(sm[1])
			if g, ok := nopadGenerics[cs]; ok {
				cs = g
			}
			return []byte(cs + "_" + string(sm[2]) + "_ci")
		})
		line = nopadBinRe.ReplaceAll(line, []byte("${1}_bin"))
		line = pageCkRe.ReplaceAll(line, nil)
		line = transRe.ReplaceAll(line, nil)
		line = rowFmtPageRe.ReplaceAll(line, nil)
		line = ariaRe.ReplaceAll(line, []byte("ENGINE=InnoDB"))
		// MariaDB names JSON/CHECK constraints after the column; MySQL needs
		// schema-unique names, so let MySQL generate them.
		line = checkNameRe.ReplaceAll(line, []byte("CHECK ("))
	}
	line = definerRe.ReplaceAll(line, []byte("DEFINER=CURRENT_USER"))
	return line, true
}
