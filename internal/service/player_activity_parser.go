package service

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
)

// PlayerActivityGrammarVersion 标识面向行的远端活动数据语法版本。
const PlayerActivityGrammarVersion = 1

const (
	playerActivityMaximumBatch    = 512
	playerActivityMaximumLine     = 16 * 1024
	playerActivityDuplicateWindow = 10 * time.Second
	// maximumPlayerActivityBatch 保留为解析器上限在 Manager 侧的别名。
	maximumPlayerActivityBatch = playerActivityMaximumBatch
)

// PlayerActivityRecord 是从一行 Minecraft 日志提取出的、已校验的带版本事件。
type PlayerActivityRecord struct {
	ProcessIdentityID model.ID
	Version           int
	SourceSequence    uint64
	Type              enums.PlayerActivityEventType
	PlayerName        string
	NormalizedName    string
	ObservedAt        time.Time
	RawEvidence       string
}

// PlayerActivityParseResult 承载有界的记录,以及关于被跳过行的非致命证据。
type PlayerActivityParseResult struct {
	Records  []PlayerActivityRecord
	Warnings []string
}

// PlayerActivityParser 解析受支持的 Vanilla/Paper/Purpur/Fabric/Forge 系日志行。
type PlayerActivityParser struct {
	DuplicateWindow time.Duration
	MaximumBatch    int
}

// NewPlayerActivityParser 创建一个带保守去重与批次上限的解析器。
func NewPlayerActivityParser() PlayerActivityParser {
	return PlayerActivityParser{DuplicateWindow: playerActivityDuplicateWindow, MaximumBatch: playerActivityMaximumBatch}
}

var (
	ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	clockPattern      = regexp.MustCompile(`\[(\d{2}):(\d{2}):(\d{2})\]`)
	dateTimePattern   = regexp.MustCompile(`(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2})`)
	joinedPattern     = regexp.MustCompile(`(?i)(?:^|\s)(?:player\s+)?([A-Za-z0-9_]{1,32})\s+joined\s+the\s+game(?:\b|$)`)
	leftPattern       = regexp.MustCompile(`(?i)(?:^|\s)(?:player\s+)?([A-Za-z0-9_]{1,32})\s+(?:left\s+the\s+game|lost\s+connection)(?:\b|$)`)
	loggedInPattern   = regexp.MustCompile(`(?i)(?:^|\s)(?:player\s+)?([A-Za-z0-9_]{1,32})\s+logged\s+in\s+with\s+entity\s+id\b`)
)

// ParseMinecraftPlayerActivity 识别一行受支持的 Minecraft 日志。
// 对普通的非玩家日志行返回 (nil, nil)。
func (p PlayerActivityParser) ParseMinecraftPlayerActivity(line string, observedAt time.Time) (*PlayerActivityRecord, error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" {
		return nil, nil
	}
	clean := strings.TrimSpace(ansiEscapePattern.ReplaceAllString(line, ""))
	var eventType enums.PlayerActivityEventType
	var name string
	switch {
	case joinedPattern.MatchString(clean), loggedInPattern.MatchString(clean):
		eventType = enums.PlayerActivityJoin
		match := joinedPattern.FindStringSubmatch(clean)
		if len(match) == 0 {
			match = loggedInPattern.FindStringSubmatch(clean)
		}
		name = match[1]
	case leftPattern.MatchString(clean):
		eventType = enums.PlayerActivityLeave
		name = leftPattern.FindStringSubmatch(clean)[1]
	default:
		return nil, nil
	}
	name = strings.TrimSpace(name)
	if !validPlayerActivityName(name) {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "玩家活动日志中的名称无效")
	}
	if observedAt.IsZero() {
		return nil, apperror.New(apperror.CodeValidationRequired, "玩家活动观测时间不能为空")
	}
	when := playerActivityTimestamp(clean, observedAt.UTC())
	return &PlayerActivityRecord{
		Version: PlayerActivityGrammarVersion, Type: eventType, PlayerName: name,
		NormalizedName: model.NormalizePlayerName(name), ObservedAt: when, RawEvidence: limitPlayerActivityEvidence(clean),
	}, nil
}

// ParseLog 解析有界的 Minecraft 原始日志内容,并抑制短窗口内的重复。
func (p PlayerActivityParser) ParseLog(content string, observedAt time.Time) PlayerActivityParseResult {
	maximum := p.MaximumBatch
	if maximum <= 0 || maximum > playerActivityMaximumBatch {
		maximum = playerActivityMaximumBatch
	}
	duplicateWindow := p.DuplicateWindow
	if duplicateWindow <= 0 {
		duplicateWindow = playerActivityDuplicateWindow
	}
	result := PlayerActivityParseResult{Records: make([]PlayerActivityRecord, 0, maximum), Warnings: make([]string, 0, 8)}
	seen := make(map[string]time.Time)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024), playerActivityMaximumLine)
	for scanner.Scan() {
		line := scanner.Text()
		record, err := p.ParseMinecraftPlayerActivity(line, observedAt)
		if err != nil {
			result.Warnings = appendBoundedWarning(result.Warnings, err.Error())
			continue
		}
		if record == nil {
			clean := strings.ToLower(ansiEscapePattern.ReplaceAllString(line, ""))
			if strings.Contains(clean, "joined the game") || strings.Contains(clean, "left the game") || strings.Contains(clean, "lost connection") || strings.Contains(clean, "logged in with entity id") {
				result.Warnings = appendBoundedWarning(result.Warnings, "不支持的玩家活动日志格式")
			}
			continue
		}
		key := record.Type.String() + "\x00" + record.NormalizedName
		if previous, ok := seen[key]; ok && absDuration(record.ObservedAt.Sub(previous)) <= duplicateWindow {
			continue
		}
		seen[key] = record.ObservedAt
		if len(result.Records) >= maximum {
			result.Warnings = appendBoundedWarning(result.Warnings, "玩家活动批次超过有界上限")
			break
		}
		record.SourceSequence = uint64(len(result.Records) + 1)
		result.Records = append(result.Records, *record)
	}
	if err := scanner.Err(); err != nil {
		result.Warnings = appendBoundedWarning(result.Warnings, "读取玩家活动日志失败: "+err.Error())
	}
	return result
}

// ParseRemoteBatch 把带版本的竖线分隔 Spool 语法解析成持久化事件。
func (p PlayerActivityParser) ParseRemoteBatch(content string, serverID, processIdentityID model.ID, fallbackObservedAt time.Time) ([]model.PlayerActivityEvent, []string, error) {
	if !serverID.Valid() || !processIdentityID.Valid() {
		return nil, nil, apperror.New(apperror.CodeValidationInvalidArgument, "玩家活动 Server 或 Process Identity 无效")
	}
	maximum := p.MaximumBatch
	if maximum <= 0 || maximum > playerActivityMaximumBatch {
		maximum = playerActivityMaximumBatch
	}
	result := make([]model.PlayerActivityEvent, 0, maximum)
	warnings := make([]string, 0, 8)
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 1024), playerActivityMaximumLine)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimSuffix(scanner.Text(), "\r"))
		if line == "" || strings.HasPrefix(line, "player_batch_") {
			continue
		}
		if strings.HasPrefix(line, "player_drop_marker=") {
			warnings = appendBoundedWarning(warnings, "远端玩家活动 Spool 发生裁剪")
			continue
		}
		record, err := parseRemotePlayerActivityLine(line, fallbackObservedAt)
		if err != nil {
			warnings = appendBoundedWarning(warnings, err.Error())
			continue
		}
		if len(result) >= maximum {
			return result, warnings, apperror.New(apperror.CodeValidationConflict, "远端玩家活动批次超过有界上限")
		}
		recordProcessID := record.ProcessIdentityID
		if !recordProcessID.Valid() {
			recordProcessID = processIdentityID
		}
		id, err := StablePlayerActivityEventID(serverID, recordProcessID, record.SourceSequence, record.Type, record.NormalizedName)
		if err != nil {
			return nil, warnings, err
		}
		now := fallbackObservedAt.UTC()
		if now.IsZero() {
			now = record.ObservedAt
		}
		result = append(result, model.PlayerActivityEvent{ID: id, ServerID: serverID, ProcessIdentityID: recordProcessID,
			SourceSequence: record.SourceSequence, Type: record.Type, PlayerName: record.PlayerName,
			NormalizedName: record.NormalizedName, RawEvidence: record.RawEvidence, ObservedAt: record.ObservedAt,
			CreatedAt: now, SchemaVersion: model.PlayerActivitySchemaVersion})
	}
	if err := scanner.Err(); err != nil {
		return nil, warnings, apperror.Wrap(apperror.CodeIOReadFailed, "读取远端玩家活动 Claim 失败", err)
	}
	return result, warnings, nil
}

// StablePlayerActivityEventID 由不可变的事件输入推导出确定性的 UUID 形态 ID。
func StablePlayerActivityEventID(serverID, processIdentityID model.ID, sequence uint64, eventType enums.PlayerActivityEventType, normalizedName string) (model.ID, error) {
	if !serverID.Valid() || !processIdentityID.Valid() || sequence == 0 || !eventType.Valid() || model.NormalizePlayerName(normalizedName) != normalizedName || normalizedName == "" {
		return "", apperror.New(apperror.CodeValidationInvalidArgument, "玩家活动 Event ID 输入无效")
	}
	payload := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s", serverID, processIdentityID, sequence, eventType, normalizedName)
	digest := sha256.Sum256([]byte(payload))
	// 在保持全部身份位确定性的同时,维持 MineOps 的 UUIDv7 形态 ID 契约。
	digest[6] = (digest[6] & 0x0f) | 0x70
	digest[8] = (digest[8] & 0x3f) | 0x80
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], digest[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], digest[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], digest[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], digest[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], digest[10:16])
	return model.ID(encoded), nil
}

func parseRemotePlayerActivityLine(line string, fallback time.Time) (PlayerActivityRecord, error) {
	fields := strings.SplitN(line, "|", 7)
	if (len(fields) != 6 && len(fields) != 7) || fields[0] != "player_activity_v1" {
		return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动记录 grammar 不支持")
	}
	recordProcessID := model.ID("")
	if len(fields) == 7 {
		recordProcessID = model.ID(strings.TrimSpace(fields[1]))
		if !recordProcessID.Valid() {
			return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动 Process Identity 无效")
		}
		fields = append(fields[:1], fields[2:]...)
	}
	sequence, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil || sequence == 0 {
		return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动序号无效")
	}
	eventType := enums.PlayerActivityEventType(fields[2])
	if !eventType.Valid() {
		return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动类型不支持")
	}
	epoch, err := strconv.ParseInt(fields[3], 10, 64)
	if err != nil || epoch <= 0 {
		return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动时间无效")
	}
	name := strings.TrimSpace(fields[4])
	if !validPlayerActivityName(name) {
		return PlayerActivityRecord{}, apperror.New(apperror.CodeValidationInvalidArgument, "远端玩家活动名称无效")
	}
	observed := time.Unix(epoch, 0).UTC()
	if observed.IsZero() {
		observed = fallback.UTC()
	}
	raw := limitPlayerActivityEvidence(fields[5])
	// 守护进程记录观测时间作为兜底;日志自带权威时钟时优先还原它。
	observed = playerActivityTimestamp(raw, observed)
	return PlayerActivityRecord{ProcessIdentityID: recordProcessID, Version: PlayerActivityGrammarVersion, SourceSequence: sequence, Type: eventType,
		PlayerName: name, NormalizedName: model.NormalizePlayerName(name), ObservedAt: observed,
		RawEvidence: raw}, nil
}

func playerActivityDroppedCount(content string) uint64 {
	var dropped uint64
	for _, line := range strings.Split(content, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "player_drop_marker=") {
			continue
		}
		fields := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(line), "player_drop_marker="), "|", 2)
		if len(fields) != 2 {
			continue
		}
		value, err := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 64)
		if err == nil {
			dropped += value
		}
	}
	return dropped
}

func playerActivityTimestamp(line string, observedAt time.Time) time.Time {
	if match := dateTimePattern.FindStringSubmatch(line); len(match) == 2 {
		for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if value, err := time.ParseInLocation(layout, match[1], time.UTC); err == nil {
				return value
			}
		}
	}
	match := clockPattern.FindStringSubmatch(line)
	if len(match) != 4 {
		return observedAt.UTC()
	}
	hour, _ := strconv.Atoi(match[1])
	minute, _ := strconv.Atoi(match[2])
	second, _ := strconv.Atoi(match[3])
	candidate := time.Date(observedAt.Year(), observedAt.Month(), observedAt.Day(), hour, minute, second, 0, time.UTC)
	// 取最接近的日期以处理跨零点的证据,避免依赖本地时区假设。
	if candidate.Sub(observedAt) > 12*time.Hour {
		candidate = candidate.Add(-24 * time.Hour)
	} else if observedAt.Sub(candidate) > 12*time.Hour {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate
}

func validPlayerActivityName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, value := range name {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' || value == '-' {
			continue
		}
		return false
	}
	return true
}

func limitPlayerActivityEvidence(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 8192 {
		return value[:8192]
	}
	return value
}

func appendBoundedWarning(warnings []string, value string) []string {
	if len(warnings) >= 16 {
		return warnings
	}
	if len(value) > 512 {
		value = value[:512]
	}
	return append(warnings, value)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
