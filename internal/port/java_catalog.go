package port

import "context"

// JDKArtifact 是与供应方无关、已核准的 Java 下载描述。
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

// JDKCatalog 解析已核准的 Java 构件,不暴露供应方响应 DTO。
type JDKCatalog interface {
	List(context.Context, int, string, string) ([]JDKArtifact, error)
	Resolve(context.Context, int, string, string) (JDKArtifact, error)
}
