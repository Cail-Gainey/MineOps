package minecraftspark

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
)

// SpikeResult 承载能力、解析器、样式清洗、可选 MSPT 与失败关闭的验证证据。
type SpikeResult struct {
	PluginVersion              string             `json:"pluginVersion"`
	ParserVersion              string             `json:"parserVersion"`
	SourceSchemaHash           string             `json:"sourceSchemaHash"`
	CollectionPriority         []CollectionMethod `json:"collectionPriority"`
	Capabilities               []Capability       `json:"capabilities"`
	CapabilitySeparation       bool               `json:"capabilitySeparation"`
	CollectionPriorityLocked   bool               `json:"collectionPriorityLocked"`
	StyledResponseParsed       bool               `json:"styledResponseParsed"`
	CappedTPSParsed            bool               `json:"cappedTPSParsed"`
	TPSWindowsParsed           bool               `json:"tpsWindowsParsed"`
	MSPTParsed                 bool               `json:"msptParsed"`
	OptionalMSPTParsed         bool               `json:"optionalMSPTParsed"`
	CrossPlatformGrammarParsed bool               `json:"crossPlatformGrammarParsed"`
	UnavailableValueRejected   bool               `json:"unavailableValueRejected"`
	UnknownVersionRejected     bool               `json:"unknownVersionRejected"`
	UnknownFormatRejected      bool               `json:"unknownFormatRejected"`
	ApacheDependencyProhibited bool               `json:"apacheDependencyProhibited"`
}

// Passed 返回全部必需的 Minecraft spark 适配器关卡是否都已通过。
func (r SpikeResult) Passed() bool {
	return r.PluginVersion == BaselinePluginVersion &&
		r.ParserVersion == ParserVersion &&
		r.SourceSchemaHash == SourceSchemaHash &&
		len(r.Capabilities) == 8 &&
		r.CapabilitySeparation &&
		r.CollectionPriorityLocked &&
		r.StyledResponseParsed &&
		r.CappedTPSParsed &&
		r.TPSWindowsParsed &&
		r.MSPTParsed &&
		r.OptionalMSPTParsed &&
		r.CrossPlatformGrammarParsed &&
		r.UnavailableValueRejected &&
		r.UnknownVersionRejected &&
		r.UnknownFormatRejected &&
		r.ApacheDependencyProhibited
}

// RunSpike 校验锁定的 Minecraft spark 支持矩阵与解析器行为。
func RunSpike() (SpikeResult, error) {
	key := AdapterKey{
		Platform:         PlatformPaper,
		Distribution:     "built-in",
		PluginVersion:    BaselinePluginVersion,
		CollectionMethod: CollectionRCONText,
		ParserVersion:    ParserVersion,
		SourceSchemaHash: SourceSchemaHash,
	}
	styledSample := "\x1b[32m" + sparkPrefix + " " + tpsHeading + "\x1b[0m\n§a" + sparkPrefix + " *20.0, 19.98, 19.95, 19.90, 19.85§r\n<gold>" + sparkPrefix + " " + currentMSPTHeading + "</gold>\n" + sparkPrefix + " 0.60/0.80/1.20/4.20; 0.70/0.90/1.40/5.20"
	report, err := ParseTPSResponse(key, styledSample)
	if err != nil {
		return SpikeResult{}, fmt.Errorf("parse locked spark response: %w", err)
	}
	withoutMSPT, err := ParseTPSResponse(key, tpsHeading+"\n20.0, 19.9, 19.8, 19.7, 19.6")
	if err != nil {
		return SpikeResult{}, fmt.Errorf("parse response without MSPT: %w", err)
	}
	spigotKey := key
	spigotKey.Platform = PlatformSpigot
	spigotReport, spigotErr := ParseTPSResponse(spigotKey, tpsHeading+"\n20.0, 19.9, 19.8, 19.7, 19.6")
	_, unavailableValueErr := ParseTPSResponse(key, tpsHeading+"\n20.0, N/A, 19.8, 19.7, 19.6")
	_, unknownVersionErr := ParseTPSResponse(AdapterKey{
		Platform:         PlatformPaper,
		Distribution:     "built-in",
		PluginVersion:    "1.11.0",
		CollectionMethod: CollectionRCONText,
		ParserVersion:    ParserVersion,
		SourceSchemaHash: SourceSchemaHash,
	}, styledSample)
	_, unknownFormatErr := ParseTPSResponse(key, "TPS: 20.0")

	capabilities := SupportMatrix()
	capabilitySeparation := false
	for _, capability := range capabilities {
		if capability.TPSSupported && !capability.MSPTSupported {
			capabilitySeparation = true
			break
		}
	}
	priority := CollectionPriority()
	apacheDependencyProhibited := true
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		for _, dependency := range buildInfo.Deps {
			path := strings.ToLower(dependency.Path)
			if strings.Contains(path, "apache") || strings.Contains(path, "hadoop") {
				apacheDependencyProhibited = false
				break
			}
		}
	}
	result := SpikeResult{
		PluginVersion:              BaselinePluginVersion,
		ParserVersion:              ParserVersion,
		SourceSchemaHash:           SourceSchemaHash,
		CollectionPriority:         priority,
		Capabilities:               capabilities,
		CapabilitySeparation:       capabilitySeparation,
		CollectionPriorityLocked:   len(priority) == 1 && priority[0] == CollectionRCONText,
		StyledResponseParsed:       report.Snapshot.MSPT != nil,
		CappedTPSParsed:            report.Snapshot.TPS.Last5Seconds == 20 && report.Snapshot.TPS.Last5SecondsCapped,
		TPSWindowsParsed:           report.Snapshot.TPS.Last10Seconds == 19.98 && report.Snapshot.TPS.Last15Minutes == 19.85,
		MSPTParsed:                 report.Snapshot.MSPT != nil && report.Snapshot.MSPT.P95 == 1.2 && report.Snapshot.MSPT.Maximum == 4.2,
		OptionalMSPTParsed:         withoutMSPT.Snapshot.MSPT == nil,
		CrossPlatformGrammarParsed: spigotErr == nil && spigotReport.Snapshot.TPS.Last1Minute == 19.8,
		UnavailableValueRejected:   errors.Is(unavailableValueErr, ErrUnsupported),
		UnknownVersionRejected:     errors.Is(unknownVersionErr, ErrUnsupported),
		UnknownFormatRejected:      errors.Is(unknownFormatErr, ErrUnsupported),
		ApacheDependencyProhibited: apacheDependencyProhibited,
	}
	return result, nil
}
