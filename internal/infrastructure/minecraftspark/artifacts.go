package minecraftspark

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Cail-Gainey/MineOps/internal/global/enums"
)

const OfficialBuildNumber = 525

// Artifact is one exact official lucko/spark build with an immutable SHA-256 digest.
type Artifact struct {
	Platform          string               `json:"platform"`
	Provider          string               `json:"provider"`
	ReleaseID         string               `json:"releaseID,omitempty"`
	Version           string               `json:"version"`
	Build             int                  `json:"build"`
	FileName          string               `json:"fileName"`
	URL               string               `json:"url"`
	SHA256            string               `json:"sha256"`
	SHA512            string               `json:"sha512"`
	Size              int64                `json:"size"`
	TargetKind        string               `json:"targetKind"`
	RequiredJavaMajor int                  `json:"requiredJavaMajor"`
	Dependencies      []DependencyArtifact `json:"dependencies,omitempty"`
}

// DependencyArtifact is one immutable runtime dependency installed alongside a Spark mod.
type DependencyArtifact struct {
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	FileName          string   `json:"fileName"`
	URL               string   `json:"url"`
	FallbackURLs      []string `json:"fallbackURLs,omitempty"`
	SHA256            string   `json:"sha256"`
	SHA512            string   `json:"sha512"`
	Size              int64    `json:"size"`
	TargetKind        string   `json:"targetKind"`
	MatchPattern      string   `json:"matchPattern"`
	RequiredJavaMajor int      `json:"requiredJavaMajor"`
}

var officialArtifacts = map[string]Artifact{
	"bukkit": {
		Platform: "bukkit", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-bukkit.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-bukkit/build/libs/spark-1.10.173-bukkit.jar", SHA256: "d2fdf73432bdee5277ab83b08b02eb27f062e7d979df1df254fbf0677e1d6fb2", TargetKind: "plugins", RequiredJavaMajor: 8,
	},
	"bungeecord": {
		Platform: "bungeecord", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-bungeecord.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-bungeecord/build/libs/spark-1.10.173-bungeecord.jar", SHA256: "20bc297fba414eec19838a5d8b99e69b3f27d8ec5b2ff99ba5405fd77438ab21", TargetKind: "plugins", RequiredJavaMajor: 8,
	},
	"forge": {
		Platform: "forge", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-forge.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-forge/build/libs/spark-1.10.173-forge.jar", SHA256: "ca09bf8ef1c481fe995acaeea5c14af1f49ef6e0f7b6f32bc061ae1454672d58", TargetKind: "mods", RequiredJavaMajor: 25,
	},
	"neoforge": {
		Platform: "neoforge", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-neoforge.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-neoforge/build/libs/spark-1.10.173-neoforge.jar", SHA256: "e81b3e0aabb47feb20b227114de8e46395393cfe4650671c26d0eec149e5e2b0", TargetKind: "mods", RequiredJavaMajor: 25,
	},
	"paper": {
		Platform: "paper", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-paper.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-paper/build/libs/spark-1.10.173-paper.jar", SHA256: "4ae58c99ee52658d059799889b8da2a1d6f3c0c60f86da699ecd7724da9486c8", TargetKind: "plugins", RequiredJavaMajor: 21,
	},
	"velocity": {
		Platform: "velocity", Provider: "spark", Version: BaselinePluginVersion, Build: OfficialBuildNumber, FileName: "spark-1.10.173-velocity.jar",
		URL: "https://ci.lucko.me/job/spark/525/artifact/spark-velocity/build/libs/spark-1.10.173-velocity.jar", SHA256: "b5c3d0f573075a95d807f8ac572e54101f7a75cd4f0bf588c79fa2f9b9874208", TargetKind: "plugins", RequiredJavaMajor: 8,
	},
}

// ArtifactForServer returns the exact approved artifact for one MineOps server family and Minecraft version.
func ArtifactForServer(serverType enums.MinecraftServerType, minecraftVersion string, javaMajor int) (Artifact, error) {
	if serverType == enums.ServerPaper && PaperUsesBundledSpark(minecraftVersion) {
		return Artifact{}, fmt.Errorf("%w: Paper Minecraft %s bundles spark; automatic external artifact installation is unavailable", ErrUnsupported, strings.TrimSpace(minecraftVersion))
	}

	platform := ""
	switch serverType {
	case enums.ServerPaper, enums.ServerPurpur:
		if javaMajor >= officialArtifacts["paper"].RequiredJavaMajor {
			platform = "paper"
		} else {
			platform = "bukkit"
		}
	case enums.ServerFolia:
		platform = "paper"
	case enums.ServerSpigot:
		platform = "bukkit"
	case enums.ServerFabric, enums.ServerForge, enums.ServerNeoForge, enums.ServerQuilt:
		if javaMajor == 0 {
			return Artifact{Platform: serverType.String(), Provider: "spark-modrinth", TargetKind: "mods"}, nil
		}
		return Artifact{}, fmt.Errorf("%w: mod loader artifact requires catalog resolution", ErrUnsupported)
	case enums.ServerBungee, enums.ServerWaterfall:
		platform = "bungeecord"
	case enums.ServerVelocity:
		platform = "velocity"
	default:
		return Artifact{}, fmt.Errorf("%w: server type %s", ErrUnsupported, serverType)
	}
	artifact, found := officialArtifacts[platform]
	if !found {
		return Artifact{}, fmt.Errorf("%w: artifact platform %s", ErrUnsupported, platform)
	}
	if javaMajor > 0 && artifact.RequiredJavaMajor > javaMajor {
		return Artifact{}, fmt.Errorf("%w: spark %s requires Java %d, server uses Java %d", ErrUnsupported, artifact.Version, artifact.RequiredJavaMajor, javaMajor)
	}
	return artifact, nil
}

// PaperUsesBundledSpark 判断指定 Minecraft 版本的 Paper 是否使用内置 Spark。
func PaperUsesBundledSpark(minecraftVersion string) bool {
	parts := strings.Split(strings.TrimSpace(minecraftVersion), ".")
	if len(parts) < 2 {
		return false
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return false
	}
	if major == 1 {
		return minor >= 21
	}
	return major >= 26
}

// PlatformForServerType returns the versioned adapter platform family used by capability and parser records.
func PlatformForServerType(serverType enums.MinecraftServerType) PlatformFamily {
	switch serverType {
	case enums.ServerPaper, enums.ServerPurpur, enums.ServerFolia:
		return PlatformPaper
	case enums.ServerSpigot:
		return PlatformSpigot
	case enums.ServerFabric:
		return PlatformFabric
	case enums.ServerQuilt:
		return PlatformQuilt
	case enums.ServerForge:
		return PlatformForge
	case enums.ServerNeoForge:
		return PlatformNeoForge
	case enums.ServerBungee, enums.ServerWaterfall:
		return PlatformBungeeCord
	case enums.ServerVelocity:
		return PlatformVelocity
	default:
		return PlatformFamily(serverType.String())
	}
}
