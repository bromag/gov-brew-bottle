package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gov-brew-bottle-creation/internal/brew"
	"gov-brew-bottle-creation/internal/report"
	"os"
	//"path"
	"path/filepath"

	"gov-brew-bottle-creation/internal/cli"
	"gov-brew-bottle-creation/internal/config"
	"gov-brew-bottle-creation/internal/fsutil"
	"gov-brew-bottle-creation/internal/hash"
	//"gov-brew-bottle-creation/internal/naming"
	"gov-brew-bottle-creation/internal/nexus"
	"gov-brew-bottle-creation/internal/plan"
	//"gov-brew-bottle-creation/internal/report"
)

func main() {
	os.Exit(run())
}

func run() int {
	// 1) Load .env (if present)
	config.LoadEnv()

	// 2) Read env config
	envCfg := config.FromEnv()

	// 3) Parse CLI flags
	cliCfg, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}

	// 4) Merge (flags override env)
	finalTag := firstNonEmpty(cliCfg.Tag, envCfg.DefaultTag)
	finalWorkdir := firstNonEmpty(cliCfg.WorkDir, envCfg.DefaultWorkdir)
	finalNexusBase := firstNonEmpty(cliCfg.NexusBase, envCfg.NexusBaseURL)

	// 5) Validate
	if finalTag == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing tag. Set --tag or DEFAULT_TAG in .env")
		return 2
	}
	if finalWorkdir == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing workdir. Set --workdir or DEFAULT_WORKDIR in .env")
		return 2
	}
	if finalNexusBase == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing nexus base. Set --nexus-base in .env")
		return 2
	}

	// Workdir sicherstellen (immer)
	if err := os.MkdirAll(finalWorkdir, 0o755); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: failed to create workdir:", err)
		return 1
	}
	ctx := context.Background()

	// --nexus-upload
	if cliCfg.NexusUpload {
		if envCfg.NexusUser == "" || envCfg.NexusPass == "" {
			_, _ = fmt.Fprintln(os.Stderr, "error: must specify NEXUS_USER and NEXUS_PASS")
			return 2
		}

		// 1) find newest bottle in finalWorkdir
		bottlePath, err := fsutil.FindBottleTarGz(finalWorkdir)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: failed to find bottle in workdir:", err)
			return 1
		}
		bottleFile := filepath.Base(bottlePath)

		// 2) derive json filename from bottle filename
		const suffix = ".bottle.tar.gz"
		if len(bottleFile) <= len(suffix) || bottleFile[len(bottleFile)-len(suffix):] != suffix {
			_, _ = fmt.Fprintln(os.Stderr, "error: bottle filename does not end with .bottle.tar.gz:", bottleFile)
			return 1
		}
		jsonFile := bottleFile[:len(bottleFile)-len(suffix)] + ".bottle.json"
		jsonPath := filepath.Join(finalWorkdir, jsonFile)

		// 3) json must exist
		if _, err := os.Stat(jsonPath); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: bottle json file not found:", jsonPath)
			return 1
		}

		up := nexus.Uploader{}

		// 4) URLs must use filenames (not local paths)
		bottleURL := joinURL(finalNexusBase, bottleFile)
		jsonURL := joinURL(finalNexusBase, jsonFile)

		// 5) Upload
		if err := up.PutFile(ctx, bottleURL, bottlePath, envCfg.NexusUser, envCfg.NexusPass); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: upload bottle:", err)
			return 1
		}
		if err := up.PutFile(ctx, jsonURL, jsonPath, envCfg.NexusUser, envCfg.NexusPass); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: upload json:", err)
			return 1
		}

		fmt.Println("upload bottle to:", bottleURL)
		fmt.Println("upload bottle json to:", jsonURL)
		return 0
	}

	// Ab hier: kein "MVP-Dry-Run-Block" mehr. Der Flow gilt immer.
	ref := cliCfg.Refs[0]

	// 6) Plan erstellen (immer)
	pl := plan.Plan(ctx, envCfg.BrewBin, ref, finalTag, finalNexusBase, joinURL)
	rep := pl.Report
	bottleName := pl.BottleName
	jsonName := pl.JSONName

	// 8) Report immer schreiben (immer)
	outPath := filepath.Join(finalWorkdir, jsonName)
	writeReport := func() int {
		f, err := os.Create(outPath) // overwrite
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: create json:", err)
			return 1
		}
		defer f.Close()

		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: write json:", err)
			return 1
		}
		return 0
	}

	if rc := writeReport(); rc != 0 {
		return rc
	}

	// 9) Wenn Plan schon failed: abbrechen (aber Report ist da)
	if rep.Status == report.StatusFailed {
		_, _ = fmt.Fprintln(os.Stderr, "error: plan failed:", rep.Error)
		fmt.Println("wrote:", outPath)
		return 1
	}

	// 10) DRY-RUN: ab hier strikt keine Side-Effects
	if cliCfg.DryRun {
		if cliCfg.BuildBottle || cliCfg.Upload {
			_, _ = fmt.Fprintln(os.Stderr, "note: --dry-run set, ignoring --build-bottle/--upload")
		}
		fmt.Println("wrote:", outPath)
		return 0
	}

	// 11) Optional: build bottle (nur ohne dry-run)
	var bottleOutPath string
	if cliCfg.BuildBottle {
		workDir, err := os.MkdirTemp(finalWorkdir, "work-")
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: failed to create temp workdir:", err)
			return 1
		}
		if !cliCfg.KeepWork {
			defer os.RemoveAll(workDir)
		} else {
			fmt.Println("keeping workdir:", workDir)
		}

		brewBin := envCfg.BrewBin

		// uninstall (Fehler: nur loggen)
		_, stderr, code, err := brew.Run(ctx, brewBin,
			[]string{"uninstall", "--ignore-dependencies", ref},
			"", nil,
		)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "warn: uninstall failed:", err)
			_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
			_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
		}

		// install --build-bottle (Fehler: abbrechen)
		_, stderr, code, err = brew.Run(ctx, brewBin,
			[]string{"install", "--build-bottle", ref},
			workDir, nil,
		)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: build-bottle failed:", err)
			_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
			_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
			return 1
		}

		// brew bottle
		_, stderr, code, err = brew.Run(ctx, brewBin,
			[]string{"bottle", "--no-rebuild", ref},
			workDir, nil,
		)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: brew bottle failed:", err)
			_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
			_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
			return 1
		}

		produced, err := fsutil.FindBottleTarGz(workDir)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: failed to find bottle:", err)
			return 1
		}

		bottleOutPath = filepath.Join(finalWorkdir, bottleName)
		if err := os.Rename(produced, bottleOutPath); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: move bottle:", err)
			return 1
		}
		fmt.Println("wrote:", bottleOutPath)

		sum, err := hash.FileSHA256(bottleOutPath)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: sha256:", err)
			return 1
		}
		rep.Sha256 = sum

		// Report nach Build überschreiben
		if rc := writeReport(); rc != 0 {
			return rc
		}
	}

	// 12) Optional: upload (nur ohne dry-run)
	if cliCfg.Upload {
		if envCfg.NexusUser == "" || envCfg.NexusPass == "" {
			_, _ = fmt.Fprintln(os.Stderr, "error: Nexus user or Nexus pass is empty")
			return 2
		}

		// wenn bottleOutPath leer ist, nimm existing aus workdir
		// (für jetzt: weiterhin require build oder stat() checks)

		if bottleOutPath == "" {
			bottleOutPath = filepath.Join(finalWorkdir, bottleName)
		}

		up := nexus.Uploader{}

		if err := up.PutFile(ctx, rep.NexusURLBottle, bottleOutPath, envCfg.NexusUser, envCfg.NexusPass); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: upload bottle:", err)
			return 1
		}

		if err := up.PutFile(ctx, rep.NexusURLJSON, outPath, envCfg.NexusUser, envCfg.NexusPass); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: upload json:", err)
			return 1
		}

		fmt.Println("upload bottle to:", rep.NexusURLBottle)
		fmt.Println("upload json to:", rep.NexusURLJSON)

		// Report nach Upload nochmals überschreiben (optional)
		if rc := writeReport(); rc != 0 {
			return rc
		}
	}

	fmt.Println("wrote:", outPath)
	return 0
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// joinURL joins base + parts with exactly one "/" between parts.
// Works even if base ends with "/" (like your Nexus URL).
func joinURL(base string, parts ...string) string {
	u := base
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	for _, p := range parts {
		for len(p) > 0 && p[0] == '/' {
			p = p[1:]
		}
		u = u + "/" + p
	}
	return u
}
