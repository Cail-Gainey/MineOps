package port

import "context"

// JDKArtifact is the provider-neutral approved Java download description.
type JDKArtifact struct {
	Version         string `json:"version"`
	MajorVersion    int    `json:"majorVersion"`
	Vendor          string `json:"vendor"`
	Architecture    string `json:"architecture"`
	OperatingSystem string `json:"operatingSystem"`
	ArchiveType     string `json:"archiveType"`
	URL             string `json:"url"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
}

// JDKCatalog resolves approved Java artifacts without exposing provider response DTOs.
type JDKCatalog interface {
	List(context.Context, int, string, string) ([]JDKArtifact, error)
	Resolve(context.Context, int, string, string) (JDKArtifact, error)
}
