package formula

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FormulaPathInRepo returns path like:
// <tapWorkdir>/Formula/s/gov-srt.rb  (folder based on name without "gov-")
func FormulaPathInRepo(tapWorkdir, formulaName string) (string, error) {
	if tapWorkdir == "" {
		return "", fmt.Errorf("tap workdir is empty")
	}

	filename := formulaName + ".rb"

	// Folder letter is based on name WITHOUT "gov-" prefix
	base := strings.TrimPrefix(formulaName, "gov-")
	if base == "" {
		base = formulaName // fallback
	}

	first := strings.ToLower(string([]rune(base)[0]))
	p := filepath.Join(tapWorkdir, "Formula", first, filename)

	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("formula file not found: %s (%v)", p, err)
	}
	return p, nil
}
