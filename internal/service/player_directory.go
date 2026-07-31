package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

const maximumPlayerDirectoryBytes = 2 * 1024 * 1024

var playerDirectoryFiles = []string{"usercache.json", "whitelist.json", "ops.json", "banned-players.json"}

type playerDirectoryEntry struct {
	UUID, Name, NormalizedName           string
	Known, Whitelisted, Operator, Banned bool
	BanReason, BanSource, BanExpiresAt   string
}

type minecraftPlayerFileRecord struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Reason  string `json:"reason"`
	Source  string `json:"source"`
	Expires string `json:"expires"`
}

// ParsePlayerDirectoryFile 解析一个有界的 Minecraft 玩家 JSON 文件。
func ParsePlayerDirectoryFile(fileName string, content []byte) ([]playerDirectoryEntry, error) {
	if len(content) > maximumPlayerDirectoryBytes {
		return nil, apperror.New(apperror.CodeValidationInvalidArgument, "Minecraft 玩家目录文件超过限制")
	}
	content = bytes.TrimPrefix(content, []byte{0xef, 0xbb, 0xbf})
	if len(bytes.TrimSpace(content)) == 0 {
		return []playerDirectoryEntry{}, nil
	}
	var records []minecraftPlayerFileRecord
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Minecraft 玩家目录 JSON 无效", err)
	}
	entries := make([]playerDirectoryEntry, 0, len(records))
	for _, record := range records {
		name := strings.TrimSpace(record.Name)
		if !validPlayerActivityName(name) {
			continue
		}
		uuid := strings.ToLower(strings.TrimSpace(record.UUID))
		if uuid != "" && len(uuid) != 36 {
			continue
		}
		entry := playerDirectoryEntry{UUID: uuid, Name: name, NormalizedName: model.NormalizePlayerName(name)}
		switch fileName {
		case "usercache.json":
			entry.Known = true
		case "whitelist.json":
			entry.Whitelisted = true
		case "ops.json":
			entry.Operator = true
		case "banned-players.json":
			entry.Banned, entry.BanReason, entry.BanSource, entry.BanExpiresAt = true, strings.TrimSpace(record.Reason), strings.TrimSpace(record.Source), strings.TrimSpace(record.Expires)
		default:
			return nil, apperror.New(apperror.CodeValidationInvalidArgument, "不支持的 Minecraft 玩家目录文件")
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// SynchronizeDirectory 刷新权威的玩家身份与权限快照。
func (m *PlayerActivityManager) SynchronizeDirectory(ctx context.Context, serverID model.ID, selectedFiles ...string) error {
	if !serverID.Valid() || m.clients == nil || m.settings == nil {
		return apperror.New(apperror.CodeValidationRequired, "玩家目录同步依赖或 Server ID 无效")
	}
	server, err := m.store.MinecraftServers().Get(ctx, serverID, false)
	if err != nil {
		return err
	}
	session, err := m.store.SSHSessions().Get(ctx, server.SSHSessionID)
	if err != nil {
		return err
	}
	client, err := m.clients.Connect(ctx, session, m.settings.Snapshot().SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	files := selectedFiles
	if len(files) == 0 {
		files = playerDirectoryFiles
	}
	entries := make(map[string]playerDirectoryEntry)
	evidence := make([]string, 0, len(files))
	for _, fileName := range files {
		if !containsPlayerDirectoryFile(fileName) {
			return apperror.New(apperror.CodeValidationInvalidArgument, "玩家目录刷新文件无效")
		}
		result, err := client.RunCommand(ctx, RemoteCommand{
			Executable: "sh", Arguments: []string{"-c", `if [ -f "$1" ]; then cat -- "$1"; fi`, "mineops-player-directory", path.Join(server.RemotePath, fileName)},
			Timeout: 10 * time.Second, MaximumOutput: maximumPlayerDirectoryBytes + 1,
		})
		if err != nil || result.Truncated || len(result.Stdout) > maximumPlayerDirectoryBytes {
			return apperror.Wrap(apperror.CodeIOReadFailed, "读取 Minecraft 玩家目录失败", err)
		}
		parsed, err := ParsePlayerDirectoryFile(fileName, []byte(result.Stdout))
		if err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(result.Stdout))
		evidence = append(evidence, fileName+":"+hex.EncodeToString(hash[:8]))
		for _, entry := range parsed {
			key := entry.UUID
			if key == "" {
				key = "name:" + entry.NormalizedName
			}
			current := entries[key]
			mergePlayerDirectoryEntry(&current, entry)
			entries[key] = current
		}
	}
	sort.Strings(evidence)
	observedAt := m.clock.Now().UTC()
	if err := m.persistDirectoryEntries(ctx, serverID, entries, strings.Join(evidence, ","), observedAt); err != nil {
		m.publish(ctx, model.PlayerEvent{Version: 1, Type: "error", ServerID: serverID, Message: err.Error(), EmittedAt: observedAt})
		return err
	}
	for _, entry := range entries {
		identity, err := m.store.Players().FindIdentity(ctx, repository.PlayerIdentityQuery{ServerID: serverID, UUID: entry.UUID, NormalizedName: entry.NormalizedName})
		if err == nil {
			m.publish(ctx, model.PlayerEvent{Version: 1, Type: "directory", ServerID: serverID, PlayerIdentityID: identity.ID, EmittedAt: observedAt})
		}
	}
	return nil
}

func (m *PlayerActivityManager) persistDirectoryEntries(ctx context.Context, serverID model.ID, entries map[string]playerDirectoryEntry, sourceVersion string, observedAt time.Time) error {
	return m.store.Transaction(ctx, func(registry repository.Registry) error {
		players := registry.Players()
		identities, err := players.ListIdentities(ctx, serverID, 200, 0)
		if err != nil {
			return err
		}
		scheduledExpiries := make(map[model.ID]string, len(identities))
		for index := range identities {
			if existing, snapshotErr := players.GetDirectorySnapshot(ctx, identities[index].ID); snapshotErr == nil && existing.Banned && parseScheduledBanExpiry(existing.BanExpiresAt) != nil {
				scheduledExpiries[identities[index].ID] = existing.BanExpiresAt
			} else if snapshotErr != nil && !isPlayerNotFound(snapshotErr) {
				return snapshotErr
			}
			snapshotID, idErr := model.NewID(observedAt)
			if idErr != nil {
				return idErr
			}
			if err := players.SaveDirectorySnapshot(ctx, &model.PlayerDirectorySnapshot{
				ID: snapshotID, ServerID: serverID, PlayerIdentityID: identities[index].ID,
				SourceVersion: sourceVersion, ObservedAt: observedAt, SchemaVersion: model.PlayerActivitySchemaVersion,
			}); err != nil {
				return err
			}
		}
		for _, entry := range entries {
			identity, err := resolveDirectoryIdentity(ctx, players, serverID, entry, observedAt)
			if err != nil {
				return err
			}
			snapshotID, err := model.NewID(observedAt)
			if err != nil {
				return err
			}
			snapshot := model.PlayerDirectorySnapshot{
				ID: snapshotID, ServerID: serverID, PlayerIdentityID: identity.ID,
				Known: entry.Known, Whitelisted: entry.Whitelisted, Operator: entry.Operator, Banned: entry.Banned,
				BanReason: entry.BanReason, BanSource: entry.BanSource, BanExpiresAt: entry.BanExpiresAt,
				SourceVersion: sourceVersion, ObservedAt: observedAt, SchemaVersion: model.PlayerActivitySchemaVersion,
			}
			if snapshot.Banned && parseScheduledBanExpiry(snapshot.BanExpiresAt) == nil {
				if scheduled := scheduledExpiries[identity.ID]; scheduled != "" {
					snapshot.BanExpiresAt = scheduled
				}
			}
			if err := players.SaveDirectorySnapshot(ctx, &snapshot); err != nil {
				return err
			}
		}
		return nil
	})
}

func resolveDirectoryIdentity(ctx context.Context, players repository.PlayerRepository, serverID model.ID, entry playerDirectoryEntry, now time.Time) (*model.PlayerIdentity, error) {
	var identity *model.PlayerIdentity
	var nameIdentity *model.PlayerIdentity
	var err error
	if entry.UUID != "" {
		identity, err = players.FindIdentity(ctx, repository.PlayerIdentityQuery{ServerID: serverID, UUID: entry.UUID})
	}
	if err == nil || isPlayerNotFound(err) {
		nameIdentity, err = players.FindIdentity(ctx, repository.PlayerIdentityQuery{ServerID: serverID, NormalizedName: entry.NormalizedName})
		if identity == nil {
			identity = nameIdentity
		}
	}
	if err != nil && !isPlayerNotFound(err) {
		return nil, err
	}
	if identity == nil {
		id, createErr := model.NewID(now)
		if createErr != nil {
			return nil, createErr
		}
		kind := enums.PlayerIdentityNameOnly
		if entry.UUID != "" {
			kind = enums.PlayerIdentityUUID
		}
		identity = &model.PlayerIdentity{ID: id, ServerID: serverID, UUID: entry.UUID, CurrentName: entry.Name, NormalizedName: entry.NormalizedName, Kind: kind, CreatedAt: now, UpdatedAt: now, SchemaVersion: model.PlayerActivitySchemaVersion}
		if err := players.CreateIdentity(ctx, identity); err != nil {
			return nil, err
		}
		return identity, nil
	}
	if nameIdentity != nil && identity.ID != nameIdentity.ID {
		if err := players.ReassignIdentity(ctx, nameIdentity.ID, identity.ID); err != nil {
			return nil, err
		}
		if err := players.DeleteIdentity(ctx, nameIdentity.ID); err != nil {
			return nil, err
		}
		if err := rebuildPlayerStatistics(ctx, players, serverID, identity.ID, now); err != nil {
			return nil, err
		}
	}
	identity.CurrentName, identity.NormalizedName, identity.UpdatedAt = entry.Name, entry.NormalizedName, now
	if entry.UUID != "" {
		identity.UUID, identity.Kind = entry.UUID, enums.PlayerIdentityUUID
	}
	if err := players.UpdateIdentity(ctx, identity); err != nil {
		return nil, err
	}
	return identity, nil
}

func rebuildPlayerStatistics(ctx context.Context, players repository.PlayerRepository, serverID, playerID model.ID, now time.Time) error {
	sessions, err := players.ListClosedSessionsForRebuild(ctx, serverID, 200, 0)
	if err != nil {
		return err
	}
	statistics := model.PlayerStatistics{PlayerIdentityID: playerID, ServerID: serverID, Accuracy: enums.PlayerAccuracyExact, UpdatedAt: now, SchemaVersion: model.PlayerActivitySchemaVersion}
	for index := range sessions {
		if sessions[index].PlayerIdentityID != playerID {
			continue
		}
		statistics.TotalDurationSeconds += sessions[index].DurationSeconds
		statistics.CompletedSessionCount++
		statistics.LongestSessionSeconds = max(statistics.LongestSessionSeconds, sessions[index].DurationSeconds)
		if statistics.FirstActivityAt == nil || sessions[index].JoinedAt.Before(*statistics.FirstActivityAt) {
			joined := sessions[index].JoinedAt
			statistics.FirstActivityAt = &joined
		}
		if sessions[index].LeftAt != nil && (statistics.LastActivityAt == nil || sessions[index].LeftAt.After(*statistics.LastActivityAt)) {
			left := *sessions[index].LeftAt
			statistics.LastActivityAt = &left
		}
		statistics.Accuracy = lessAccuratePlayerState(statistics.Accuracy, sessions[index].Accuracy)
	}
	if statistics.CompletedSessionCount == 0 {
		return nil
	}
	return players.SaveStatistics(ctx, &statistics)
}

func mergePlayerDirectoryEntry(target *playerDirectoryEntry, source playerDirectoryEntry) {
	if target.Name == "" || source.Known {
		target.UUID, target.Name, target.NormalizedName = source.UUID, source.Name, source.NormalizedName
	}
	target.Known = target.Known || source.Known
	target.Whitelisted = target.Whitelisted || source.Whitelisted
	target.Operator = target.Operator || source.Operator
	target.Banned = target.Banned || source.Banned
	if source.Banned {
		target.BanReason, target.BanSource, target.BanExpiresAt = source.BanReason, source.BanSource, source.BanExpiresAt
	}
}

func containsPlayerDirectoryFile(value string) bool {
	for _, candidate := range playerDirectoryFiles {
		if value == candidate {
			return true
		}
	}
	return false
}

// ManagePlayer 发送一条已校验的 Minecraft 命令并刷新权威名录状态。
func (m *PlayerActivityManager) ManagePlayer(ctx context.Context, action string, input model.PlayerActionInput) (model.PlayerOverview, error) {
	if err := input.Validate(); err != nil {
		return model.PlayerOverview{}, err
	}
	server, err := m.store.MinecraftServers().Get(ctx, input.ServerID, false)
	if err != nil {
		return model.PlayerOverview{}, err
	}
	if server.State != enums.LifecycleRunning || m.processes == nil {
		return model.PlayerOverview{}, apperror.New(apperror.CodeValidationConflict, "玩家管理操作要求 Server 正在运行")
	}
	if action == "ban" && input.ExpiresAt != nil && !input.ExpiresAt.After(m.clock.Now().UTC()) {
		return model.PlayerOverview{}, apperror.New(apperror.CodeValidationInvalidArgument, "封禁到期时间必须晚于当前时间")
	}
	identity, err := m.PlayerDetail(ctx, input.ServerID, input.PlayerIdentityID)
	if err != nil {
		return model.PlayerOverview{}, err
	}
	processIdentity, err := m.store.ProcessIdentities().GetByServer(ctx, input.ServerID)
	if err != nil {
		return model.PlayerOverview{}, err
	}
	command, refreshDirectory, err := playerManagementCommand(action, identity.Name, input)
	if err != nil {
		return model.PlayerOverview{}, err
	}
	if err := m.processes.Attach(*processIdentity).Input(ctx, []byte(command+"\n")); err != nil {
		return model.PlayerOverview{}, err
	}
	if refreshDirectory {
		var syncErr error
		for attempt := 0; attempt < 4; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return model.PlayerOverview{}, ctx.Err()
				case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
				}
			}
			syncErr = m.SynchronizeDirectory(ctx, input.ServerID)
			if syncErr == nil {
				break
			}
		}
		if syncErr != nil {
			return model.PlayerOverview{}, apperror.Wrap(apperror.CodeIOReadFailed, "玩家命令已执行，但目录同步失败", syncErr)
		}
	}
	if action == "ban" && input.ExpiresAt != nil {
		if err := m.saveScheduledBanExpiry(ctx, input.PlayerIdentityID, input.ExpiresAt.UTC()); err != nil {
			return model.PlayerOverview{}, err
		}
	}
	return m.PlayerDetail(ctx, input.ServerID, input.PlayerIdentityID)
}

func (m *PlayerActivityManager) saveScheduledBanExpiry(ctx context.Context, playerID model.ID, expiresAt time.Time) error {
	snapshot, err := m.store.Players().GetDirectorySnapshot(ctx, playerID)
	if err != nil {
		return err
	}
	snapshot.BanExpiresAt = expiresAt.Format(time.RFC3339)
	snapshot.ObservedAt = m.clock.Now().UTC()
	return m.store.Players().SaveDirectorySnapshot(ctx, snapshot)
}

func (m *PlayerActivityManager) expireServerBans(ctx context.Context, serverID model.ID) error {
	identities, err := m.store.Players().ListIdentities(ctx, serverID, 200, 0)
	if err != nil {
		return err
	}
	now := m.clock.Now().UTC()
	for index := range identities {
		snapshot, snapshotErr := m.store.Players().GetDirectorySnapshot(ctx, identities[index].ID)
		if snapshotErr != nil {
			if isPlayerNotFound(snapshotErr) {
				continue
			}
			return snapshotErr
		}
		expiresAt := parseScheduledBanExpiry(snapshot.BanExpiresAt)
		if !snapshot.Banned || expiresAt == nil || expiresAt.After(now) {
			continue
		}
		if _, err := m.ManagePlayer(ctx, "pardon", model.PlayerActionInput{ServerID: serverID, PlayerIdentityID: identities[index].ID}); err != nil {
			return err
		}
	}
	return nil
}

func parseScheduledBanExpiry(value string) *time.Time {
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	parsed = parsed.UTC()
	return &parsed
}

func playerManagementCommand(action, name string, input model.PlayerActionInput) (string, bool, error) {
	if !validPlayerActivityName(name) {
		return "", false, apperror.New(apperror.CodeValidationInvalidArgument, "玩家名称无效")
	}
	switch action {
	case "add_whitelist":
		return "whitelist add " + name, true, nil
	case "remove_whitelist":
		return "whitelist remove " + name, true, nil
	case "grant_operator":
		return "op " + name, true, nil
	case "revoke_operator":
		return "deop " + name, true, nil
	case "ban":
		command := "ban " + name
		if reason := strings.TrimSpace(input.Reason); reason != "" {
			command += " " + strings.ReplaceAll(reason, "\n", " ")
		}
		return command, true, nil
	case "pardon":
		return "pardon " + name, true, nil
	case "kick":
		command := "kick " + name
		if reason := strings.TrimSpace(input.Reason); reason != "" {
			command += " " + strings.ReplaceAll(reason, "\n", " ")
		}
		return command, false, nil
	default:
		return "", false, apperror.New(apperror.CodeValidationInvalidArgument, "玩家管理操作无效")
	}
}
