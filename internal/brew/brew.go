package brew

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

type Client struct {
	BrewPath string // "brew" oder "/opt/homebrew/bin/brew"
}

type infoV2 struct {
	Formulae []struct {
		Versions struct {
			Stable string `json:"stable"`
		} `json:"versions"`
	} `json:"formulae"`
}

func (c Client) FormulaVersion(ctx context.Context, ref string) (string, error) {
	brew := c.BrewPath
	if brew == "" {
		brew = "brew"
	}

	cmd := exec.CommandContext(ctx, brew, "info", "--json=v2", ref)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("brew info failed: %w (stderr=%q)", err, stderr.String())
	}

	raw := stdout.Bytes()

	// gov-brew prints a banner before the JSON. Find the first '{' and parse from there.
	i := bytes.IndexByte(raw, '{')
	if i < 0 {
		return "", fmt.Errorf("no JSON found in output (stdout=%q stderr=%q)", stdout.String(), stderr.String())
	}
	raw = raw[i:]

	var parsed infoV2
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("parse brew info json: %w", err)
	}

	if len(parsed.Formulae) == 0 || parsed.Formulae[0].Versions.Stable == "" {
		return "", fmt.Errorf("no stable version found for %q", ref)
	}

	return parsed.Formulae[0].Versions.Stable, nil
}
