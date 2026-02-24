package plan

import (
	"context"
	"gov-brew-bottle-creation/internal/brew"
	"gov-brew-bottle-creation/internal/naming"
	"gov-brew-bottle-creation/internal/report"
	"path"
)

type Result struct {
	Report     report.BottleReport
	BottleName string
	JSONName   string
}

// Plan erzeugen den Report + Namen/URLs, ohne Build/Upload
//joinURL wird die Funktion hinzugefügt, damit plan kein main importieren muss.

func Plan(
	ctx context.Context,
	brewBin string,
	ref string,
	tag string,
	nexusBase string,
	joinURL func(base string, parts ...string) string,
) Result {
	short := path.Base(ref)

	bc := brew.Client{BrewPath: brewBin}
	version, verr := bc.FormulaVersion(ctx, ref)
	if verr != nil {
		version = "unknown"
	}

	bottleName := naming.BottleTarGz(short, version, tag)
	jsonName := naming.BottleJSON(short, version, tag)

	rep := report.BottleReport{
		Ref:     ref,
		Formula: short,
		Version: version,
		Tag:     tag,

		BottleFile: bottleName,
		JSONFile:   jsonName,

		NexusURLBottle: joinURL(nexusBase, bottleName),
		NexusURLJSON:   joinURL(nexusBase, jsonName),

		Status: report.StatusPlanned,
	}

	if verr != nil {
		rep.Status = report.StatusFailed
		rep.Error = verr.Error()
	}

	return Result{
		Report:     rep,
		BottleName: bottleName,
		JSONName:   jsonName,
	}
}
