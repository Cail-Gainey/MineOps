package model

import (
	"bufio"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// JavaRuntimeSource identifies how a remote Java installation was discovered or managed.
type JavaRuntimeSource string

const (
	JavaSourceManaged  JavaRuntimeSource = "managed"
	JavaSourceJavaHome JavaRuntimeSource = "java_home"
	JavaSourcePath     JavaRuntimeSource = "path"
	JavaSourceSystem   JavaRuntimeSource = "system"
	JavaSourceManual   JavaRuntimeSource = "manual"
)

// JavaRuntime is one reusable remote Java installation bound to an SSH Session.
type JavaRuntime struct {
	ID           ID                `json:"id"`
	SSHSessionID ID                `json:"sshSessionID"`
	Version      string            `json:"version"`
	MajorVersion int               `json:"majorVersion"`
	Vendor       string            `json:"vendor"`
	Architecture string            `json:"architecture"`
	Source       JavaRuntimeSource `json:"source"`
	InstallPath  string            `json:"installPath"`
	JavaHome     string            `json:"javaHome"`
	Managed      bool              `json:"managed"`
	Default      bool              `json:"default"`
	Reusable     bool              `json:"reusable"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

// JavaVersionInfo contains parsed `java -XshowSettings:properties -version` properties.
type JavaVersionInfo struct {
	Version      string `json:"version"`
	MajorVersion int    `json:"majorVersion"`
	Vendor       string `json:"vendor"`
	Architecture string `json:"architecture"`
	JavaHome     string `json:"javaHome"`
}

// JavaRequirement contains the supported range and preferred Java major for one server version.
type JavaRequirement struct {
	MinimumMajor   int `json:"minimumMajor"`
	MaximumMajor   int `json:"maximumMajor"`
	PreferredMajor int `json:"preferredMajor"`
}

var quotedJavaVersion = regexp.MustCompile(`(?i)(?:java|openjdk)(?: full)? version "([^"]+)"`)
var plainJavaVersion = regexp.MustCompile(`(?i)^(?:java|openjdk)\s+(?:version\s+)?["']?([0-9][^\s"']*)`)

// NewJavaRuntime creates a validated remote Java record with an ordered ID.
func NewJavaRuntime(clock Clock, runtime JavaRuntime) (*JavaRuntime, error) {
	if clock == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Clock 不能为空")
	}
	now := clock.Now().UTC()
	id, err := NewID(now)
	if err != nil {
		return nil, apperror.Wrap(apperror.CodeInternal, "生成 Java Runtime ID 失败", err)
	}
	runtime.ID = id
	runtime.CreatedAt = now
	runtime.UpdatedAt = now
	if err := runtime.Validate(); err != nil {
		return nil, err
	}
	return &runtime, nil
}

// Validate checks Java identity, remote paths, and source values without network access.
func (r JavaRuntime) Validate() error {
	if !r.SSHSessionID.Valid() || strings.TrimSpace(r.Version) == "" || r.MajorVersion < 1 {
		return apperror.New(apperror.CodeValidationRequired, "Java Runtime SSH、版本和主版本不能为空")
	}
	if !path.IsAbs(r.InstallPath) || !path.IsAbs(r.JavaHome) {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Java Runtime 路径必须是远程 POSIX 绝对路径")
	}
	if r.Source != JavaSourceManaged && r.Source != JavaSourceJavaHome && r.Source != JavaSourcePath && r.Source != JavaSourceSystem && r.Source != JavaSourceManual {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Java Runtime 来源无效")
	}
	return nil
}

// ParseJavaVersionOutput parses Java 8, modern OpenJDK, vendor, architecture, and Java Home output.
func ParseJavaVersionOutput(output string) (JavaVersionInfo, error) {
	info := JavaVersionInfo{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if match := quotedJavaVersion.FindStringSubmatch(line); len(match) == 2 && info.Version == "" {
			info.Version = match[1]
		}
		if match := plainJavaVersion.FindStringSubmatch(line); len(match) == 2 && info.Version == "" {
			info.Version = match[1]
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.TrimSpace(key) {
		case "java.version":
			info.Version = value
		case "java.runtime.version", "java.specification.version":
			if info.Version == "" {
				info.Version = value
			}
		case "java.vendor":
			info.Vendor = value
		case "os.arch":
			info.Architecture = value
		case "java.home":
			info.JavaHome = path.Clean(value)
		}
	}
	if scanner.Err() != nil {
		return JavaVersionInfo{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "解析 Java 版本输出失败", scanner.Err())
	}
	major, err := parseJavaMajor(info.Version)
	if err != nil {
		return JavaVersionInfo{}, err
	}
	info.MajorVersion = major
	if info.Vendor == "" {
		info.Vendor = inferJavaVendor(output)
	}
	if info.Architecture == "" {
		info.Architecture = "unknown"
	}
	if info.JavaHome == "." {
		info.JavaHome = ""
	}
	return info, nil
}

// JavaRequirementForMinecraft returns the centralized versioned Java compatibility policy.
func JavaRequirementForMinecraft(serverType, minecraftVersion string) (JavaRequirement, error) {
	major, minor, patchVersion, err := parseMinecraftVersion(minecraftVersion)
	if err != nil {
		return JavaRequirement{}, apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft 版本格式无效")
	}
	_ = serverType
	if major == 26 {
		return JavaRequirement{MinimumMajor: 25, MaximumMajor: 25, PreferredMajor: 25}, nil
	}
	if major != 1 {
		return JavaRequirement{}, apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft 版本格式无效")
	}
	switch {
	case minor <= 16:
		return JavaRequirement{MinimumMajor: 8, MaximumMajor: 8, PreferredMajor: 8}, nil
	case minor == 17:
		return JavaRequirement{MinimumMajor: 16, MaximumMajor: 17, PreferredMajor: 17}, nil
	case minor < 20 || minor == 20 && patchVersion <= 4:
		return JavaRequirement{MinimumMajor: 17, MaximumMajor: 21, PreferredMajor: 17}, nil
	default:
		return JavaRequirement{MinimumMajor: 21, MaximumMajor: 25, PreferredMajor: 21}, nil
	}
}

// Compatible reports whether a Java major falls inside the requirement range.
func (r JavaRequirement) Compatible(javaMajor int) bool {
	return javaMajor >= r.MinimumMajor && javaMajor <= r.MaximumMajor
}

func parseJavaMajor(version string) (int, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Java 版本输出缺少版本号")
	}
	parts := strings.Split(version, ".")
	majorText := parts[0]
	if majorText == "1" && len(parts) > 1 {
		majorText = parts[1]
	}
	majorDigits := strings.TrimLeftFunc(majorText, func(character rune) bool { return character < '0' || character > '9' })
	majorDigits = strings.TrimRightFunc(majorDigits, func(character rune) bool { return character < '0' || character > '9' })
	major, err := strconv.Atoi(majorDigits)
	if err != nil || major < 1 {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "Java 主版本无法识别")
	}
	return major, nil
}

func inferJavaVendor(output string) string {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, "temurin") || strings.Contains(lower, "adoptium"):
		return "Eclipse Adoptium"
	case strings.Contains(lower, "corretto"):
		return "Amazon Corretto"
	case strings.Contains(lower, "graalvm"):
		return "GraalVM"
	case strings.Contains(lower, "oracle"):
		return "Oracle"
	case strings.Contains(lower, "openjdk"):
		return "OpenJDK"
	default:
		return "Unknown"
	}
}

func parseMinecraftVersion(version string) (int, int, int, error) {
	parts := strings.Split(strings.TrimSpace(version), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, strconv.ErrSyntax
	}
	values := []int{0, 0, 0}
	for index, part := range parts {
		digits := strings.TrimRightFunc(part, func(character rune) bool { return character < '0' || character > '9' })
		value, err := strconv.Atoi(digits)
		if err != nil {
			return 0, 0, 0, err
		}
		values[index] = value
	}
	return values[0], values[1], values[2], nil
}
