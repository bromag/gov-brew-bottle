package cli

import (
	"flag"
	"fmt"
	"io"
)

type Config struct {
	Refs    []string
	Tag     string
	WorkDir string
	DryRun  bool

	NexusBase   string
	NexusUser   string
	NexusPass   string
	NexusPrefix string
	NexusUpload bool

	BuildBottle bool
	Upload      bool
	KeepWork    bool

	UpdateFormula bool

	TapGitURL    string
	TapGitBranch string

	TapWorkdir string
}

type multiString []string

func (m *multiString) String() string { return "" }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func ParseFlags(args []string) (Config, error) {
	fs := flag.NewFlagSet("gov-brew-bottler", flag.ContinueOnError)

	fs.SetOutput(io.Discard)

	var refs multiString
	fs.Var(&refs, "ref", "reference to tag")

	tag := fs.String("tag", "", "tag")
	workDir := fs.String("work-dir", "", "work directory")
	dryRun := fs.Bool("dry-run", false, "dry run: create json only")

	nBase := fs.String("nexus-base", "", "nexus base")
	nUser := fs.String("nexus-user", "", "nexus user")
	nPass := fs.String("nexus-pass", "", "nexus password")
	nPrefix := fs.String("nexus-prefix", "", "nexus prefix")

	buildBottle := fs.Bool("build-bottle", false, "build bottle (install --build-bottle + bottle)")
	upload := fs.Bool("upload", false, "upload bottle (.bottle.tar.gz) and json (.bottle.json) to Nexus")
	keepWork := fs.Bool("keep-work", false, "keep work dir")

	nexusUpload := fs.Bool("nexus-upload", false, "upload existing bottle/json from workdir to Nexus (if alredy build)")

	updateFormula := fs.Bool("update-formula", false, "update Formula bottle block based on dist/*.bottle.json")

	TapGitURL := fs.String("tap-git-url", "", "tap git url")
	TapGitBranch := fs.String("tap-git-branch", "", "tap branch")

	tapWorkdir := fs.String("tap-workdir", "", "path to local tap git repo (where Formula/ lives)")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Refs:          []string(refs),
		Tag:           *tag,
		WorkDir:       *workDir,
		DryRun:        *dryRun,
		NexusBase:     *nBase,
		NexusUser:     *nUser,
		NexusPass:     *nPass,
		NexusPrefix:   *nPrefix,
		NexusUpload:   *nexusUpload,
		BuildBottle:   *buildBottle,
		Upload:        *upload,
		KeepWork:      *keepWork,
		UpdateFormula: *updateFormula,
		TapGitURL:     *TapGitURL,
		TapGitBranch:  *TapGitBranch,
		TapWorkdir:    *tapWorkdir,
	}

	// Upload triggert auch --build-bottle
	if cfg.Upload && !cfg.BuildBottle {
		cfg.BuildBottle = true
	}

	// keep work mach ohne build keinen Sinn (bei Upload habe ich build auf true gesetzt!)
	if cfg.KeepWork && !cfg.BuildBottle {
		cfg.KeepWork = false
	}

	// --nexus-upload gewinnt immer (upload-only, ohne build, ohne dry-run)
	if cfg.NexusUpload {
		cfg.Upload = true       // damit dein Upload-Block im main läuft
		cfg.BuildBottle = false // hart aus: niemals bauen
		cfg.DryRun = false      // sonst würde  Upload ignoriert werden

		// Optional: keep-work macht hier keinen Sinn, weil nichts gebaut wird
		cfg.KeepWork = false
	}

	//ref required unless upload-only
	if !cfg.NexusUpload && len(cfg.Refs) == 0 {
		return Config{}, fmt.Errorf("reference must be specified (use --ref)")
	}

	return cfg, nil
}
