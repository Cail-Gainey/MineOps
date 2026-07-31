package model

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// PropertyLineKind 标识一条被原样保留的 server.properties 物理行。
type PropertyLineKind string

const (
	PropertyBlank   PropertyLineKind = "blank"
	PropertyComment PropertyLineKind = "comment"
	PropertyEntry   PropertyLineKind = "property"
	PropertyUnknown PropertyLineKind = "unknown"
)

// PropertyLine 保留原始物理行,并在可安全识别时附带解析出的键值。
type PropertyLine struct {
	Kind  PropertyLineKind `json:"kind"`
	Raw   string           `json:"raw"`
	Key   string           `json:"key,omitempty"`
	Value string           `json:"value,omitempty"`
}

// PropertiesDocument 完整保留注释、空行、顺序、重复键、未知行与换行风格。
type PropertiesDocument struct {
	Lines           []PropertyLine `json:"lines"`
	Newline         string         `json:"newline"`
	TrailingNewline bool           `json:"trailingNewline"`
}

// ParseProperties 为 Java properties 内容建立无损的物理行模型。
func ParseProperties(content string) PropertiesDocument {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	trailing := strings.HasSuffix(normalized, "\n")
	physicalLines := strings.Split(normalized, "\n")
	if trailing && len(physicalLines) > 0 {
		physicalLines = physicalLines[:len(physicalLines)-1]
	}
	lines := make([]PropertyLine, len(physicalLines))
	continued := false
	for index, raw := range physicalLines {
		line := parsePropertyLine(raw, continued || hasContinuation(raw))
		lines[index] = line
		continued = hasContinuation(raw)
	}
	return PropertiesDocument{Lines: lines, Newline: newline, TrailingNewline: trailing}
}

// Render 重建文档,未改动的物理行逐字保留。
func (d PropertiesDocument) Render() string {
	newline := d.Newline
	if newline != "\r\n" {
		newline = "\n"
	}
	values := make([]string, len(d.Lines))
	for index, line := range d.Lines {
		values[index] = line.Raw
	}
	result := strings.Join(values, newline)
	if d.TrailingNewline {
		result += newline
	}
	return result
}

// Get 返回某个键的最后一个取值,与 Java Properties 的重复键语义一致。
func (d PropertiesDocument) Get(key string) (string, bool) {
	for index := len(d.Lines) - 1; index >= 0; index-- {
		if d.Lines[index].Kind == PropertyEntry && d.Lines[index].Key == key {
			return d.Lines[index].Value, true
		}
	}
	return "", false
}

// Set 只更新最后一处匹配项或追加新项,不改写无关文本。
func (d *PropertiesDocument) Set(key, value string) error {
	if d == nil || strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\x00") || strings.ContainsAny(value, "\r\n\x00") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Properties Key 或 Value 无效")
	}
	raw := encodeProperty(key, true) + "=" + encodeProperty(value, false)
	for index := len(d.Lines) - 1; index >= 0; index-- {
		if d.Lines[index].Kind == PropertyEntry && d.Lines[index].Key == key {
			d.Lines[index] = PropertyLine{Kind: PropertyEntry, Raw: raw, Key: key, Value: value}
			return nil
		}
	}
	d.Lines = append(d.Lines, PropertyLine{Kind: PropertyEntry, Raw: raw, Key: key, Value: value})
	return nil
}

// ServerPort 返回已校验的 server-port 值,缺失时返回 Minecraft 默认端口。
func (d PropertiesDocument) ServerPort() (uint16, error) {
	value, exists := d.Get("server-port")
	if !exists || strings.TrimSpace(value) == "" {
		return 25565, nil
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port < 1 || port > 65535 {
		return 0, apperror.New(apperror.CodeValidationInvalidArgument, "server-port 必须在 1 到 65535 之间")
	}
	return uint16(port), nil
}

// VersionToken 返回保守远端保存前使用的内容与修改时间标识。
func (d PropertiesDocument) VersionToken(modifiedAt time.Time) string {
	return RemoteTextVersionToken([]byte(d.Render()), modifiedAt)
}

func parsePropertyLine(raw string, continued bool) PropertyLine {
	trimmed := strings.TrimLeft(raw, " \t\f")
	if trimmed == "" {
		return PropertyLine{Kind: PropertyBlank, Raw: raw}
	}
	if continued || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
		kind := PropertyUnknown
		if !continued {
			kind = PropertyComment
		}
		return PropertyLine{Kind: kind, Raw: raw}
	}
	keyStart := len(raw) - len(trimmed)
	separator := propertySeparatorIndex(raw, keyStart)
	if separator < 0 {
		key, err := decodeProperty(strings.TrimSpace(raw))
		if err != nil || key == "" {
			return PropertyLine{Kind: PropertyUnknown, Raw: raw}
		}
		return PropertyLine{Kind: PropertyEntry, Raw: raw, Key: key}
	}
	rawKey := strings.TrimSpace(raw[keyStart:separator])
	valueStart := separator
	if raw[separator] == '=' || raw[separator] == ':' {
		valueStart++
	} else {
		for valueStart < len(raw) && (raw[valueStart] == ' ' || raw[valueStart] == '\t' || raw[valueStart] == '\f') {
			valueStart++
		}
		if valueStart < len(raw) && (raw[valueStart] == '=' || raw[valueStart] == ':') {
			valueStart++
		}
	}
	for valueStart < len(raw) && (raw[valueStart] == ' ' || raw[valueStart] == '\t' || raw[valueStart] == '\f') {
		valueStart++
	}
	key, keyErr := decodeProperty(rawKey)
	value, valueErr := decodeProperty(raw[valueStart:])
	if keyErr != nil || valueErr != nil || key == "" {
		return PropertyLine{Kind: PropertyUnknown, Raw: raw}
	}
	return PropertyLine{Kind: PropertyEntry, Raw: raw, Key: key, Value: value}
}

func propertySeparatorIndex(value string, start int) int {
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '=' || character == ':' || character == ' ' || character == '\t' || character == '\f' {
			return index
		}
	}
	return -1
}

func hasContinuation(value string) bool {
	backslashes := 0
	for index := len(value) - 1; index >= 0 && value[index] == '\\'; index-- {
		backslashes++
	}
	return backslashes%2 == 1
}

func decodeProperty(value string) (string, error) {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' || index+1 >= len(value) {
			result.WriteByte(value[index])
			continue
		}
		index++
		switch value[index] {
		case 't':
			result.WriteByte('\t')
		case 'n':
			result.WriteByte('\n')
		case 'r':
			result.WriteByte('\r')
		case 'f':
			result.WriteByte('\f')
		case 'u':
			if index+4 >= len(value) {
				return "", strconv.ErrSyntax
			}
			code, err := strconv.ParseUint(value[index+1:index+5], 16, 16)
			if err != nil {
				return "", err
			}
			result.WriteRune(utf16.Decode([]uint16{uint16(code)})[0])
			index += 4
		default:
			result.WriteByte(value[index])
		}
	}
	return result.String(), nil
}

func encodeProperty(value string, key bool) string {
	var result strings.Builder
	for index, character := range value {
		switch character {
		case '\\':
			result.WriteString("\\\\")
		case '\t':
			result.WriteString("\\t")
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\f':
			result.WriteString("\\f")
		case '=', ':', '#', '!':
			if key || index == 0 {
				result.WriteByte('\\')
			}
			result.WriteRune(character)
		case ' ':
			if key || index == 0 {
				result.WriteString("\\ ")
			} else {
				result.WriteRune(character)
			}
		default:
			result.WriteRune(character)
		}
	}
	return result.String()
}
