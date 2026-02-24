package formula

import (
	"fmt"
	"strings"
)

// ref: owner/tap/formula
func ParseRef(ref string) (tap string, formula string, err error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid ref %q, expected owner/tap/formula", ref)
	}
	return parts[0] + "/" + parts[1], parts[2], nil
}
