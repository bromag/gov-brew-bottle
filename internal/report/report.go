package report

type Status string

const (
	StatusPlanned Status = "planned"
	StatusFailed  Status = "failed"
)

type BottleReport struct {
	Ref     string `json:"ref"`
	Formula string `json:"formula"`
	Version string `json:"version"`
	Tag     string `json:"tag"`

	BottleFile string `json:"bottle_file"`
	JSONFile   string `json:"json_file"`

	NexusURLBottle string `json:"nexus_url_bottle,omitempty"`
	NexusURLJSON   string `json:"nexus_url_json,omitempty"`

	Status Status `json:"status"`
	Error  string `json:"error,omitempty"`

	Sha256 string `json:"sha256,omitempty"`
}
