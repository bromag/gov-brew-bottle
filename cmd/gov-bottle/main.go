package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gov-brew-bottle-creation/internal/brew"
	"os"
	"path"
	"path/filepath"

	"gov-brew-bottle-creation/internal/cli"
	"gov-brew-bottle-creation/internal/config"
	"gov-brew-bottle-creation/internal/fsutil"
	"gov-brew-bottle-creation/internal/hash"
	"gov-brew-bottle-creation/internal/naming"
	"gov-brew-bottle-creation/internal/nexus"
	"gov-brew-bottle-creation/internal/report"
)

func main() {
	// 1) Load .env (if present)
	config.LoadEnv()

	// 2) Read env config
	envCfg := config.FromEnv()

	// 3) Parse CLI flags
	cliCfg, err := cli.ParseFlags(os.Args[1:])
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// 4) Merge (flags override env)
	finalTag := firstNonEmpty(cliCfg.Tag, envCfg.DefaultTag)
	finalWorkdir := firstNonEmpty(cliCfg.WorkDir, envCfg.DefaultWorkdir)

	finalNexusBase := firstNonEmpty(cliCfg.NexusBase, envCfg.NexusBaseURL)
	//finalNexusPrefix := firstNonEmpty(cliCfg.NexusPrefix, envCfg.NexusPrefix)

	// 5) Validate the merged config (for dry-run we only need tag/workdir)
	if finalTag == "" {
		fmt.Fprintln(os.Stderr, "error: missing tag. Set --tag or DEFAULT_TAG in .env")
		os.Exit(2)
	}
	if finalWorkdir == "" {
		_, _ = fmt.Fprintln(os.Stderr, "error: missing workdir. Set --workdir or DEFAULT_WORKDIR in .env")
		os.Exit(2)
	}

	// ---- DRY-RUN MVP: create friend-style JSON report in ./dist ----
	if cliCfg.DryRun {
		ref := cliCfg.Refs[0]
		short := path.Base(ref)

		bc := brew.Client{BrewPath: envCfg.BrewBin}
		version, verr := bc.FormulaVersion(context.Background(), ref)
		if verr != nil {
			version = "unknown"
		}

		bottleName := naming.BottleTarGz(short, version, finalTag)
		jsonName := naming.BottleJSON(short, version, finalTag)

		if err := os.MkdirAll(finalWorkdir, 0o755); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: failed to create workdir:", err)
			os.Exit(1)
		}

		rep := report.BottleReport{
			Ref:     ref,
			Formula: short,
			Version: version,
			Tag:     finalTag,

			BottleFile: bottleName,
			JSONFile:   jsonName,

			NexusURLBottle: joinURL(finalNexusBase, bottleName),
			NexusURLJSON:   joinURL(finalNexusBase, jsonName),

			//NexusURLBottle: joinURL(finalNexusBase, finalNexusPrefix, short, version, bottleName),
			//NexusURLJSON:   joinURL(finalNexusBase, finalNexusPrefix, short, version, jsonName),

			Status: report.StatusPlanned,
		}

		if verr != nil {
			rep.Status = report.StatusFailed
			rep.Error = verr.Error()
		}

		// Optional: build bottle
		var bottleOutPath string
		if cliCfg.BuildBottle {
			workDir, err := os.MkdirTemp(finalWorkdir, "work-")
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: failed to create workdir:", err)
				os.Exit(1)
			}
			if !cliCfg.KeepWork {
				defer os.RemoveAll(workDir)
			} else {
				fmt.Println("keeping workdir:", workDir)
			}

			brewBin := "gov-brew" // momentan hart: später wird es aus envCfg gelesen
			_, stderr, code, err := brew.Run(context.Background(), brewBin,
				[]string{"install", "--build-bottle", ref},
				"", nil,
			)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: build-bottle failed:", err)
				_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
				_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
				os.Exit(1)
			}

			// Create bottle in the temp workdir
			_, stderr, code, err = brew.Run(context.Background(), brewBin,
				[]string{"bottle", "--no-rebuild", ref},
				workDir, nil,
			)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: brew bottle failed:", err)
				_, _ = fmt.Fprintln(os.Stderr, "stderr:", stderr)
				_, _ = fmt.Fprintln(os.Stderr, "exit:", code)
				os.Exit(1)
			}

			// find generated bottle tar.gz
			produced, err := fsutil.FindBottleTarGz(workDir)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: failed to find bottle:", err)
				os.Exit(1)
			}
			// Move/rename to freind format into finalWorkdir
			bottleOutPath = filepath.Join(finalWorkdir, bottleName)
			if err := os.Rename(produced, bottleOutPath); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: move bottle:", err)
				os.Exit(1)
			}

			// Compute sha256 for json
			sum, err := hash.FileSHA256(bottleOutPath)
			if err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "error: sha256:", err)
				os.Exit(1)
			}
			rep.Sha256 = sum
		}

		outPath := filepath.Join(finalWorkdir, jsonName)
		f, err := os.Create(outPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: create json:", err)
			os.Exit(1)
		}
		defer f.Close()

		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "error: write json:", err)
			os.Exit(1)
		}

		if cliCfg.UploadJSON {
			if envCfg.NexusUser == "" || envCfg.NexusPass == "" {
				_, _ = fmt.Fprintln(os.Stderr, "error: Nexus user, Nexus pass or Nexus password is empty")
				os.Exit(2)
			}

			up := nexus.Uploader{}
			if err := up.PutFile(
				context.Background(),
				rep.NexusURLJSON,
				outPath,
				envCfg.NexusUser,
				envCfg.NexusPass,
			); err != nil {
				fmt.Fprintln(os.Stderr, "error: upload nexus:", err)
				os.Exit(1)
			}

			fmt.Println("upload json to: ", rep.NexusURLJSON)
		}

		fmt.Printf("tag=%s\nworkdir=%s\nnexus=%s\n", finalTag, finalWorkdir, finalNexusBase)
		fmt.Println("wrote:", outPath)
		return
	}

	// Non-dry-run not implemented yet
	fmt.Printf("tag=%s\nworkdir=%s\nnexus=%s\n", finalTag, finalWorkdir, finalNexusBase)
	_, _ = fmt.Fprintln(os.Stderr, "non-dry-run not implemented yet")
	os.Exit(3)
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
