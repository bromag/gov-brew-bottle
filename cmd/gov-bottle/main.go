package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gov-brew-bottle-creation/internal/brew"
	"gov-brew-bottle-creation/internal/cli"
	"gov-brew-bottle-creation/internal/config"
	"gov-brew-bottle-creation/internal/formula"
	"gov-brew-bottle-creation/internal/fsutil"
	"gov-brew-bottle-creation/internal/hash"
	"gov-brew-bottle-creation/internal/nexus"
	"gov-brew-bottle-creation/internal/plan"
	"gov-brew-bottle-creation/internal/report"
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
	finalTapWorkdir := firstNonEmpty(cliCfg.TapWorkdir, envCfg.TapWorkdir)

	// 5) Validate
	if finalTag == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing tag. Set --tag or DEFAULT_TAG in .env")
		return 2
	}
	if finalWorkdir == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing workdir. Set --work-dir or DEFAULT_WORKDIR in .env")
		return 2
	}
	if finalNexusBase == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing nexus base. Set --nexus-base or NEXUS_BASE_URL in .env")
		return 2
	}

	// Workdir sicherstellen (immer)
	if err := os.MkdirAll(finalWorkdir, 0o755); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: failed to create workdir:", err)
		return 1
	}

	ctx := context.Background()

	// ------------------------------------------------------------
	// Upload-only: --nexus-upload (kein ref nötig, kein plan, kein build)
	// ------------------------------------------------------------
	if cliCfg.NexusUpload {
		if envCfg.NexusUser == "" || envCfg.NexusPass == "" {
			_, _ = fmt.Fprintln(os.Stderr, "error: must specify NEXUS_USER and NEXUS_PASS")
			return 2
		}

		bottlePath, err := fsutil.FindBottleTarGz(finalWorkdir)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: failed to find bottle in workdir:", err)
			return 1
		}
		bottleFile := filepath.Base(bottlePath)

		const suffix = ".bottle.tar.gz"
		if len(bottleFile) <= len(suffix) || bottleFile[len(bottleFile)-len(suffix):] != suffix {
			_, _ = fmt.Fprintln(os.Stderr, "error: bottle filename does not end with .bottle.tar.gz:", bottleFile)
			return 1
		}

		jsonFile := bottleFile[:len(bottleFile)-len(suffix)] + ".bottle.json"
		jsonPath := filepath.Join(finalWorkdir, jsonFile)

		if _, err := os.Stat(jsonPath); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: bottle json file not found:", jsonPath)
			return 1
		}

		bottleURL := joinURL(finalNexusBase, bottleFile)
		jsonURL := joinURL(finalNexusBase, jsonFile)

		up := nexus.Uploader{}

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

	// ------------------------------------------------------------
	// Normal flow (ref benötigt)
	// ------------------------------------------------------------
	if len(cliCfg.Refs) == 0 {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing --ref")
		return 2
	}
	ref := cliCfg.Refs[0]

	// Plan erstellen
	pl := plan.Plan(ctx, envCfg.BrewBin, ref, finalTag, finalNexusBase, joinURL)
	rep := pl.Report
	bottleName := pl.BottleName
	jsonName := pl.JSONName

	// Report schreiben helper
	outPath := filepath.Join(finalWorkdir, jsonName)
	writeReport := func() int {
		f, err := os.Create(outPath)
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

	// initial report
	if rc := writeReport(); rc != 0 {
		return rc
	}

	// Plan failed?
	if rep.Status == report.StatusFailed {
		_, _ = fmt.Fprintln(os.Stderr, "error: plan failed:", rep.Error)
		fmt.Println("wrote:", outPath)
		return 1
	}

	// dry-run: keine side effects
	if cliCfg.DryRun {
		if cliCfg.BuildBottle || cliCfg.Upload {
			_, _ = fmt.Fprintln(os.Stderr, "note: --dry-run set, ignoring --build-bottle/--upload")
		}
		fmt.Println("wrote:", outPath)
		return 0
	}

	// Optional: build bottle
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

		// uninstall (Fehler nur loggen)
		_, stderr, code, err := brew.Run(ctx, brewBin,
			[]string{"uninstall", "--ignore-dependencies", ref},
			"", nil,
		)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "warn: uninstall failed:", err)
			_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
			_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
		}

		// install --build-bottle
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

		// OPTIONAL: Formula updaten (NACH build + sha)
		if rc := maybeUpdateFormula(ref, finalWorkdir, finalTapWorkdir, finalNexusBase, cliCfg.UpdateFormula); rc != 0 {
			return rc
		}
	}

	// Optional: upload
	if cliCfg.Upload {
		if envCfg.NexusUser == "" || envCfg.NexusPass == "" {
			_, _ = fmt.Fprintln(os.Stderr, "error: Nexus user or Nexus pass is empty")
			return 2
		}

		// wenn nicht gebaut in diesem Run: nehme existing aus workdir
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

		// optional: report nochmals überschreiben
		if rc := writeReport(); rc != 0 {
			return rc
		}
	}

	fmt.Println("wrote:", outPath)
	return 0
}

func maybeUpdateFormula(ref, workdir, tapWorkdir, rootURL string, update bool) int {
	if !update {
		return 0
	}
	if tapWorkdir == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: --update-formula requires --tap-workdir (or TAP_WORKDIR)")
		return 2
	}

	_, name, err := formula.ParseRef(ref) // tlchmi/ch-gov-brew/gov-srt -> gov-srt
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: parse ref:", err)
		return 1
	}

	formulaPath, err := formula.FormulaPathInRepo(tapWorkdir, name)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	shas, err := formula.CollectShasFromWorkdir(workdir, name)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: collect shas:", err)
		return 1
	}

	if err := formula.ReplaceBottleBlock(formulaPath, rootURL, shas); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error: update bottle block:", err)
		return 1
	}

	fmt.Println("updated formula:", formulaPath)
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
