package minecraftspark

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	// BaselinePluginVersion is the current lucko/spark version accepted by the locked text parser.
	BaselinePluginVersion = "1.10.173"
	// Fabric12111PluginVersion is the last official spark Fabric build compatible with Minecraft 1.21.11.
	Fabric12111PluginVersion = "1.10.170"
	// ParserVersion identifies the immutable fallback grammar exercised by the stage 0 spike.
	ParserVersion = "rcon-tps/2"
	// SourceSchemaHash identifies the exact headings and field order accepted by the fallback grammar.
	SourceSchemaHash = "a6cdbbb392e4d5579fa9cd450777f9c796c35cc109ffaa8518dee21953315d34"
)

// SupportsPluginVersion reports whether the locked parser grammar supports the detected spark version series.
func SupportsPluginVersion(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 || parts[0] != "1" || parts[1] != "10" {
		return false
	}
	_, err := strconv.Atoi(parts[2])
	return err == nil
}

// ErrUnsupported marks an unknown plugin version, platform grammar, or response format.
var ErrUnsupported = errors.New("unsupported Minecraft spark response")

// PlatformFamily identifies a Minecraft server or proxy implementation family.
type PlatformFamily string

// CollectionMethod identifies a versioned Minecraft spark data source.
type CollectionMethod string

const (
	PlatformPaper      PlatformFamily = "paper"
	PlatformSpigot     PlatformFamily = "spigot"
	PlatformFabric     PlatformFamily = "fabric"
	PlatformForge      PlatformFamily = "forge"
	PlatformNeoForge   PlatformFamily = "neoforge"
	PlatformSponge     PlatformFamily = "sponge-api-12"
	PlatformBungeeCord PlatformFamily = "bungeecord"
	PlatformVelocity   PlatformFamily = "velocity"
	PlatformQuilt      PlatformFamily = "quilt"

	CollectionBridgeAPI CollectionMethod = "bridge-api"
	CollectionHealthRaw CollectionMethod = "health-raw"
	CollectionRCONText  CollectionMethod = "rcon-text"
)

// Capability records installation mode and independently detected TPS and MSPT support.
type Capability struct {
	Platform       PlatformFamily `json:"platform"`
	Distribution   string         `json:"distribution"`
	TPSSupported   bool           `json:"tpsSupported"`
	MSPTSupported  bool           `json:"msptSupported"`
	InitialSupport string         `json:"initialSupport"`
}

// AdapterKey selects a parser using platform, distribution, exact plugin version, and parser version.
type AdapterKey struct {
	Platform         PlatformFamily   `json:"platform"`
	Distribution     string           `json:"distribution"`
	PluginVersion    string           `json:"pluginVersion"`
	CollectionMethod CollectionMethod `json:"collectionMethod"`
	ParserVersion    string           `json:"parserVersion"`
	SourceSchemaHash string           `json:"sourceSchemaHash"`
}

// CollectRequest describes the detected platform and selected versioned collection method.
type CollectRequest struct {
	Platform         PlatformFamily   `json:"platform"`
	Distribution     string           `json:"distribution"`
	PluginVersion    string           `json:"pluginVersion"`
	CollectionMethod CollectionMethod `json:"collectionMethod"`
}

// Collector is the stage 0 contract for collecting one versioned Minecraft spark report.
type Collector interface {
	Collect(context.Context, CollectRequest) (Report, error)
}

// TPSWindows contains the fixed windows emitted by the supported spark TPS grammar.
type TPSWindows struct {
	Last5Seconds        float64 `json:"last5Seconds"`
	Last5SecondsCapped  bool    `json:"last5SecondsCapped"`
	Last10Seconds       float64 `json:"last10Seconds"`
	Last10SecondsCapped bool    `json:"last10SecondsCapped"`
	Last1Minute         float64 `json:"last1Minute"`
	Last1MinuteCapped   bool    `json:"last1MinuteCapped"`
	Last5Minutes        float64 `json:"last5Minutes"`
	Last5MinutesCapped  bool    `json:"last5MinutesCapped"`
	Last15Minutes       float64 `json:"last15Minutes"`
	Last15MinutesCapped bool    `json:"last15MinutesCapped"`
}

// MSPTDistribution contains the optional tick-duration distribution emitted by supported platforms.
type MSPTDistribution struct {
	Minimum float64 `json:"minimum"`
	Median  float64 `json:"median"`
	P95     float64 `json:"p95"`
	Maximum float64 `json:"maximum"`
}

// Snapshot contains one parsed TPS sample and an optional MSPT distribution.
type Snapshot struct {
	TPS  TPSWindows        `json:"tps"`
	MSPT *MSPTDistribution `json:"mspt,omitempty"`
}

// Report binds a parsed snapshot to the exact adapter contract that produced it.
type Report struct {
	Adapter  AdapterKey `json:"adapter"`
	Snapshot Snapshot   `json:"snapshot"`
}

// SupportMatrix returns the stage 0 Minecraft spark platform capability baseline.
func SupportMatrix() []Capability {
	return []Capability{
		{Platform: PlatformPaper, Distribution: "built-in", TPSSupported: true, MSPTSupported: true, InitialSupport: "core"},
		{Platform: PlatformPaper, Distribution: "bukkit-plugin", TPSSupported: true, MSPTSupported: true, InitialSupport: "core"},
		{Platform: PlatformSpigot, Distribution: "bukkit-plugin", TPSSupported: true, MSPTSupported: false, InitialSupport: "core"},
		{Platform: PlatformFabric, Distribution: "mod", TPSSupported: true, MSPTSupported: true, InitialSupport: "core"},
		{Platform: PlatformForge, Distribution: "mod", TPSSupported: true, MSPTSupported: true, InitialSupport: "core"},
		{Platform: PlatformNeoForge, Distribution: "mod", TPSSupported: true, MSPTSupported: true, InitialSupport: "core"},
		{Platform: PlatformBungeeCord, Distribution: "proxy-plugin", TPSSupported: false, MSPTSupported: false, InitialSupport: "detect-only"},
		{Platform: PlatformVelocity, Distribution: "proxy-plugin", TPSSupported: false, MSPTSupported: false, InitialSupport: "detect-only"},
	}
}

// CollectionPriority returns the currently implemented collection methods in priority order.
func CollectionPriority() []CollectionMethod {
	return []CollectionMethod{CollectionRCONText}
}
