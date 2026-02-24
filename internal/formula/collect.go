package formula

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gov-brew-bottle-creation/internal/report"
)

// Sammelt alle sha256 Einträge aus dist/<formula>-*.bottle.json
func CollectShasFromWorkdir(workdir, formulaName string) (map[string]string, error) {
	pattern := filepath.Join(workdir, fmt.Sprintf("%s-*.bottle.json", formulaName))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no bottle.json reports found: %s", pattern)
	}

	out := map[string]string{}
	for _, p := range matches {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var rep report.BottleReport
		if err := json.Unmarshal(b, &rep); err != nil {
			continue
		}
		// Nur valide Einträge
		if strings.TrimSpace(rep.Tag) == "" || strings.TrimSpace(rep.Sha256) == "" {
			continue
		}
		out[rep.Tag] = rep.Sha256
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no sha256 entries found in reports (missing sha256?)")
	}
	return out, nil
}
