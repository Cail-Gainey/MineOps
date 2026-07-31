package minecraftspark

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

const (
	// BaselinePluginVersion 是锁定文本解析器当前接受的 lucko/spark 版本。
	BaselinePluginVersion = "1.10.173"
	// Fabric12111PluginVersion 是兼容 Minecraft 1.21.11 的最后一个官方 spark Fabric 构建。
	Fabric12111PluginVersion = "1.10.170"
	// ParserVersion 标识基线验证所使用的不可变兜底语法。
	ParserVersion = "rcon-tps/2"
	// SourceSchemaHash 标识兜底语法接受的确切标题与字段顺序。
	SourceSchemaHash = "a6cdbbb392e4d5579fa9cd450777f9c796c35cc109ffaa8518dee21953315d34"
)

// SupportsPluginVersion 返回锁定的解析语法是否支持探测到的 spark 版本系列。
func SupportsPluginVersion(version string) bool {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) != 3 || parts[0] != "1" || parts[1] != "10" {
		return false
	}
	_, err := strconv.Atoi(parts[2])
	return err == nil
}

// ErrUnsupported 标记未知的插件版本、平台语法或响应格式。
var ErrUnsupported = errors.New("unsupported Minecraft spark response")

// PlatformFamily 标识一个 Minecraft 服务端或代理端实现家族。
type PlatformFamily string

// CollectionMethod 标识一个带版本的 Minecraft spark 数据来源。
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

// Capability 记录安装方式,以及独立探测得到的 TPS 与 MSPT 支持情况。
type Capability struct {
	Platform       PlatformFamily `json:"platform"`
	Distribution   string         `json:"distribution"`
	TPSSupported   bool           `json:"tpsSupported"`
	MSPTSupported  bool           `json:"msptSupported"`
	InitialSupport string         `json:"initialSupport"`
}

// AdapterKey 用平台、发行版、确切插件版本与解析器版本选定一个解析器。
type AdapterKey struct {
	Platform         PlatformFamily   `json:"platform"`
	Distribution     string           `json:"distribution"`
	PluginVersion    string           `json:"pluginVersion"`
	CollectionMethod CollectionMethod `json:"collectionMethod"`
	ParserVersion    string           `json:"parserVersion"`
	SourceSchemaHash string           `json:"sourceSchemaHash"`
}

// CollectRequest 描述探测到的平台与选定的带版本采集方式。
type CollectRequest struct {
	Platform         PlatformFamily   `json:"platform"`
	Distribution     string           `json:"distribution"`
	PluginVersion    string           `json:"pluginVersion"`
	CollectionMethod CollectionMethod `json:"collectionMethod"`
}

// Collector 是采集一份带版本 Minecraft spark 报告的契约。
type Collector interface {
	Collect(context.Context, CollectRequest) (Report, error)
}

// TPSWindows 承载受支持 spark TPS 语法输出的固定时间窗。
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

// MSPTDistribution 承载受支持平台输出的可选 tick 时长分布。
type MSPTDistribution struct {
	Minimum float64 `json:"minimum"`
	Median  float64 `json:"median"`
	P95     float64 `json:"p95"`
	Maximum float64 `json:"maximum"`
}

// Snapshot 承载一条解析出的 TPS 样本与可选的 MSPT 分布。
type Snapshot struct {
	TPS  TPSWindows        `json:"tps"`
	MSPT *MSPTDistribution `json:"mspt,omitempty"`
}

// Report 把解析出的 Snapshot 与产出它的确切适配器契约绑定。
type Report struct {
	Adapter  AdapterKey `json:"adapter"`
	Snapshot Snapshot   `json:"snapshot"`
}

// SupportMatrix 返回 Minecraft spark 平台能力的基线矩阵。
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

// CollectionPriority 按优先级返回当前已实现的采集方式。
func CollectionPriority() []CollectionMethod {
	return []CollectionMethod{CollectionRCONText}
}
