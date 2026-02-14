package brew

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

func Run(ctx context.Context, bin string, args []string, dir string, env map[string]string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	//Env optional
	if len(env) > 0 {
		e := cmd.Environ()
		for k, v := range env {
			e = append(e, k+"="+v)
		}
		cmd.Env = e
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Exit code
		exit := 1
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		}
		return stdout.String(), stderr.String(), exit, fmt.Errorf("run failed: %w (cmd=%q)", err, cmd.String())
	}
	return stdout.String(), stderr.String(), 0, nil
}
