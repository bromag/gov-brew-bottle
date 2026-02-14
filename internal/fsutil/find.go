package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func FindBottleTarGZ(dir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.bottle.tar.gz"))
	if err != nil {
		return "", fmt.Errorf("glob: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no bottle tar.gz found in %s", dir)
	}
	if len(matches) == 1 {
		return matches[0], nil
	}

	// Wen mehrere: nimm die neuste robust bei edge cases
	type fi struct {
		path string
		t    time.Time
	}
	list := make([]fi, 0, len(matches))
	for _, p := range matches {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		list = append(list, fi{path: p, t: st.ModTime()})
	}
	if len(list) == 0 {
		return "", fmt.Errorf("mustliple bottles found, but cannto stat them")
	}
	sort.Slice(list, func(i, j int) bool { return list[i].t.After(list[j].t) })
	return list[0].path, nil
}
