package formula

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

// Start: optional spaces, optional "#", spaces, "bottle do"
var reBottleStart = regexp.MustCompile(`^\s*(?:#\s*)?bottle\s+do\s*$`)

// End: optional spaces, optional "#", spaces, "end" (optional comment)
var reBottleEnd = regexp.MustCompile(`^\s*(?:#\s*)?end\s*(?:#.*)?$`)

// Insert fallback: before depends_on
var reDependsOn = regexp.MustCompile(`^\s*depends_on\b`)

// ReplaceBottleBlock replaces an existing bottle block (commented OR uncommented)
// with a fresh, uncommented block built from shas.
// If no bottle block exists, it inserts before the first "depends_on" line.
func ReplaceBottleBlock(formulaPath string, rootURL string, shas map[string]string) error {
	in, err := os.ReadFile(formulaPath)
	if err != nil {
		return fmt.Errorf("read formula: %w", err)
	}

	lines, err := readLines(in)
	if err != nil {
		return fmt.Errorf("read lines: %w", err)
	}

	// 1) find bottle start/end (commented or not)
	startIdx := -1
	endIdx := -1
	for i, ln := range lines {
		if startIdx < 0 && reBottleStart.MatchString(ln) {
			startIdx = i
			continue
		}
		if startIdx >= 0 && reBottleEnd.MatchString(ln) {
			endIdx = i
			break
		}
	}

	newBlockLines := buildBottleBlockLines(rootURL, shas) // <-- FIX: 2 args

	if startIdx >= 0 {
		if endIdx < 0 {
			return fmt.Errorf("found bottle start but cannot find bottle end in %s", formulaPath)
		}

		// Replace inclusive [startIdx..endIdx]
		out := make([]string, 0, len(lines)-(endIdx-startIdx+1)+len(newBlockLines))
		out = append(out, lines[:startIdx]...)
		out = append(out, newBlockLines...)
		out = append(out, lines[endIdx+1:]...)
		return writeLines(formulaPath, out)
	}

	// 2) No bottle block found -> insert before first depends_on (fallback)
	insertAt := -1
	for i, ln := range lines {
		if reDependsOn.MatchString(ln) {
			insertAt = i
			break
		}
	}

	if insertAt < 0 {
		// As very last fallback: append before file end
		out := append(lines, "")
		out = append(out, newBlockLines...)
		return writeLines(formulaPath, out)
	}

	out := make([]string, 0, len(lines)+len(newBlockLines)+1)
	out = append(out, lines[:insertAt]...)
	// keep a blank line before depends_on if not already
	if insertAt > 0 && strings.TrimSpace(lines[insertAt-1]) != "" {
		out = append(out, "")
	}
	out = append(out, newBlockLines...)
	out = append(out, "")
	out = append(out, lines[insertAt:]...)
	return writeLines(formulaPath, out)
}

func buildBottleBlockLines(rootURL string, shas map[string]string) []string {
	keys := make([]string, 0, len(shas))
	for k := range shas {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	cellFor := func(platform string) string {
		if strings.Contains(platform, "linux") {
			return ":any_skip_relocation"
		}
		return ":any"
	}

	out := []string{"  bottle do"}

	// root_url nur schreiben, wenn gesetzt
	if strings.TrimSpace(rootURL) != "" {
		out = append(out, fmt.Sprintf(`    root_url "%s"`, rootURL))
	}

	for _, platform := range keys {
		sha := shas[platform]
		out = append(out, fmt.Sprintf(
			`    sha256 cellar: %s, %s: "%s"`,
			cellFor(platform),
			platform,
			sha,
		))
	}

	out = append(out, "  end")
	return out
}

func readLines(b []byte) ([]string, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	// allow long lines
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 2*1024*1024)

	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

func writeLines(path string, lines []string) error {
	// keep final newline
	out := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return fmt.Errorf("write formula: %w", err)
	}
	return nil
}
