package cli

import (
	"flag"
	"fmt"
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

	UploadJSON bool

	BuildBottle bool
	Upload      bool
	KeepWork    bool
}

type multiString []string

func (m *multiString) String() string { return "" }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func ParseFlags(args []string) (Config, error) {
	fs := flag.NewFlagSet("gov-brew-bottler", flag.ContinueOnError)

	var refs multiString
	fs.Var(&refs, "ref", "reference to tag")

	tag := fs.String("tag", "", "tag")
	workDir := fs.String("work-dir", "", "work directory")
	dryRun := fs.Bool("dry-run", false, "dry run: create json only")
	uploadJSON := fs.Bool("upload-json", false, "Upload the generated .bottle.json to Nexus (dry-run)")

	nBase := fs.String("nexus-base", "", "nexus base")
	nUser := fs.String("nexus-user", "", "nexus user")
	nPass := fs.String("nexus-pass", "", "nexus password")
	nPrefix := fs.String("nexus-prefix", "", "nexus prefix")

	buildBottle := fs.Bool("build-bottle", false, "build bottle (install --build-bottle + bottle)")
	upload := fs.Bool("upload", false, "upload bottle (install --upload bottle)")
	keepWork := fs.Bool("keep-work", false, "keep work dir")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	cfg := Config{
		Refs:        []string(refs),
		Tag:         *tag,
		WorkDir:     *workDir,
		DryRun:      *dryRun,
		NexusBase:   *nBase,
		NexusUser:   *nUser,
		NexusPass:   *nPass,
		NexusPrefix: *nPrefix,
		UploadJSON:  *uploadJSON,
		BuildBottle: *buildBottle,
		Upload:      *upload,
		KeepWork:    *keepWork,
	}

	if len(cfg.Refs) == 0 {
		return Config{}, fmt.Errorf("reference must be specified")
	}
	return cfg, nil
}
