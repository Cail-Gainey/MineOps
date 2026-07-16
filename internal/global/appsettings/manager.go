package appsettings

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
	"github.com/Cail-Gainey/MineOps/internal/global/enums"
	"github.com/Cail-Gainey/MineOps/internal/model"
	"github.com/Cail-Gainey/MineOps/internal/repository"
)

// Change describes one committed settings snapshot update.
type Change struct {
	Category enums.SettingsCategory
	Snapshot model.SettingsSnapshot
}

// Subscriber receives committed settings changes synchronously after the transaction succeeds.
type Subscriber func(Change)

// Manager is the sole owner of the in-memory Settings Snapshot and encrypted persistence updates.
type Manager struct {
	store repository.Store

	mu          sync.Mutex
	snapshot    atomic.Pointer[model.SettingsSnapshot]
	rawPayloads map[enums.SettingsCategory][]byte
	subscribers map[uint64]Subscriber
	nextID      uint64
}

// NewManager creates a Settings Manager initialized with embedded defaults.
func NewManager(store repository.Store) (*Manager, error) {
	if store == nil {
		return nil, apperror.New(apperror.CodeValidationRequired, "Settings Store 不能为空")
	}
	manager := &Manager{
		store: store, rawPayloads: make(map[enums.SettingsCategory][]byte), subscribers: make(map[uint64]Subscriber),
	}
	defaults := model.DefaultSettings()
	manager.snapshot.Store(&defaults)
	return manager, nil
}

// Load replaces defaults with every valid persisted category and rejects corrupt settings.
func (m *Manager) Load(ctx context.Context) (model.SettingsSnapshot, error) {
	payloads, err := m.store.Settings().Load(ctx)
	if err != nil {
		return model.SettingsSnapshot{}, err
	}
	snapshot := model.DefaultSettings()
	for category, payload := range payloads {
		if err := decodeCategory(&snapshot, category, payload); err != nil {
			return model.SettingsSnapshot{}, apperror.Wrap(apperror.CodeValidationInvalidArgument, "Settings Category 数据损坏", err).WithDetails(map[string]any{
				"category": category,
			})
		}
	}
	for index := range snapshot.Downloads.Sources {
		source := &snapshot.Downloads.Sources[index]
		if source.Official && source.Provider == "papermc" && strings.EqualFold(strings.TrimRight(source.BaseURL, "/"), "https://api.papermc.io") {
			source.BaseURL = "https://fill.papermc.io"
			source.ProbeURL = "https://fill.papermc.io/v3/projects/paper"
		}
		if source.Official && source.Provider == "forge" && strings.HasSuffix(source.ProbeURL, "/net/minecraftforge/forge/maven-metadata.json") {
			source.ProbeURL = strings.TrimSuffix(source.ProbeURL, "maven-metadata.json") + "maven-metadata.xml"
		}
	}
	if err := snapshot.Validate(); err != nil {
		return model.SettingsSnapshot{}, err
	}
	m.mu.Lock()
	m.rawPayloads = clonePayloads(payloads)
	m.snapshot.Store(&snapshot)
	m.mu.Unlock()
	return snapshot, nil
}

// Snapshot returns the current immutable settings value.
func (m *Manager) Snapshot() model.SettingsSnapshot {
	if snapshot := m.snapshot.Load(); snapshot != nil {
		return *snapshot
	}
	return model.DefaultSettings()
}

// Save validates and transactionally persists the complete snapshot while preserving unknown category fields.
func (m *Manager) Save(ctx context.Context, snapshot model.SettingsSnapshot) error {
	snapshot.SchemaVersion = model.SettingsSchemaVersion
	if err := snapshot.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	previous := m.Snapshot()
	changed := changedCategories(previous, snapshot)
	payloads, err := encodeCategories(snapshot, m.rawPayloads)
	if err != nil {
		m.mu.Unlock()
		return apperror.Wrap(apperror.CodeInternal, "编码 Settings 失败", err)
	}
	if err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		for category, payload := range payloads {
			if err := registry.Settings().Save(ctx, category, model.SettingsSchemaVersion, payload); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		m.mu.Unlock()
		return err
	}
	m.rawPayloads = clonePayloads(payloads)
	m.snapshot.Store(&snapshot)
	subscribers := copySubscribers(m.subscribers)
	m.mu.Unlock()
	for _, category := range changed {
		for _, subscriber := range subscribers {
			subscriber(Change{Category: category, Snapshot: snapshot})
		}
	}
	return nil
}

func changedCategories(previous, current model.SettingsSnapshot) []enums.SettingsCategory {
	pairs := []struct {
		category enums.SettingsCategory
		previous any
		current  any
	}{
		{enums.SettingsGeneral, previous.General, current.General},
		{enums.SettingsTheme, previous.Theme, current.Theme},
		{enums.SettingsPaths, previous.Paths, current.Paths},
		{enums.SettingsMirrors, previous.Mirrors, current.Mirrors},
		{enums.SettingsLogging, previous.Logging, current.Logging},
		{enums.SettingsMonitoring, previous.Monitoring, current.Monitoring},
		{enums.SettingsFirewall, previous.Firewall, current.Firewall},
		{enums.SettingsLayout, previous.Layout, current.Layout},
		{enums.SettingsSSH, previous.SSH, current.SSH},
		{enums.SettingsTerminal, previous.Terminal, current.Terminal},
		{enums.SettingsDownloads, previous.Downloads, current.Downloads},
		{enums.SettingsStorage, previous.Storage, current.Storage},
	}
	changed := make([]enums.SettingsCategory, 0, len(pairs))
	for _, pair := range pairs {
		if !reflect.DeepEqual(pair.previous, pair.current) {
			changed = append(changed, pair.category)
		}
	}
	return changed
}

// ResetCategory restores one category to embedded defaults and commits the complete valid snapshot.
func (m *Manager) ResetCategory(ctx context.Context, category enums.SettingsCategory) error {
	if !category.Valid() {
		return apperror.New(apperror.CodeValidationInvalidArgument, "Settings Category 无效")
	}
	snapshot := m.Snapshot()
	defaults := model.DefaultSettings()
	applyDefaultCategory(&snapshot, defaults, category)
	if err := snapshot.Validate(); err != nil {
		return err
	}
	payloads, err := encodeCategories(snapshot, nil)
	if err != nil {
		return apperror.Wrap(apperror.CodeInternal, "编码默认 Settings Category 失败", err)
	}
	m.mu.Lock()
	if err := m.store.Transaction(ctx, func(registry repository.Registry) error {
		return registry.Settings().Save(ctx, category, model.SettingsSchemaVersion, payloads[category])
	}); err != nil {
		m.mu.Unlock()
		return err
	}
	m.rawPayloads[category] = append([]byte(nil), payloads[category]...)
	m.snapshot.Store(&snapshot)
	subscribers := copySubscribers(m.subscribers)
	m.mu.Unlock()
	for _, subscriber := range subscribers {
		subscriber(Change{Category: category, Snapshot: snapshot})
	}
	return nil
}

// Subscribe registers a controlled synchronous hot-update observer and returns its unsubscribe function.
func (m *Manager) Subscribe(subscriber Subscriber) func() {
	if subscriber == nil {
		return func() {}
	}
	m.mu.Lock()
	m.nextID++
	id := m.nextID
	m.subscribers[id] = subscriber
	m.mu.Unlock()
	return func() {
		m.mu.Lock()
		delete(m.subscribers, id)
		m.mu.Unlock()
	}
}

func decodeCategory(snapshot *model.SettingsSnapshot, category enums.SettingsCategory, payload []byte) error {
	switch category {
	case enums.SettingsGeneral:
		return json.Unmarshal(payload, &snapshot.General)
	case enums.SettingsTheme:
		return json.Unmarshal(payload, &snapshot.Theme)
	case enums.SettingsPaths:
		return json.Unmarshal(payload, &snapshot.Paths)
	case enums.SettingsMirrors:
		return json.Unmarshal(payload, &snapshot.Mirrors)
	case enums.SettingsLogging:
		return json.Unmarshal(payload, &snapshot.Logging)
	case enums.SettingsMonitoring:
		return json.Unmarshal(payload, &snapshot.Monitoring)
	case enums.SettingsFirewall:
		return json.Unmarshal(payload, &snapshot.Firewall)
	case enums.SettingsLayout:
		return json.Unmarshal(payload, &snapshot.Layout)
	case enums.SettingsSSH:
		return json.Unmarshal(payload, &snapshot.SSH)
	case enums.SettingsTerminal:
		return json.Unmarshal(payload, &snapshot.Terminal)
	case enums.SettingsDownloads:
		return json.Unmarshal(payload, &snapshot.Downloads)
	case enums.SettingsStorage:
		return json.Unmarshal(payload, &snapshot.Storage)
	default:
		return apperror.New(apperror.CodeValidationInvalidArgument, "未知 Settings Category")
	}
}

func encodeCategories(snapshot model.SettingsSnapshot, existing map[enums.SettingsCategory][]byte) (map[enums.SettingsCategory][]byte, error) {
	values := map[enums.SettingsCategory]any{
		enums.SettingsGeneral: snapshot.General, enums.SettingsTheme: snapshot.Theme, enums.SettingsPaths: snapshot.Paths,
		enums.SettingsMirrors: snapshot.Mirrors, enums.SettingsLogging: snapshot.Logging,
		enums.SettingsMonitoring: snapshot.Monitoring, enums.SettingsFirewall: snapshot.Firewall,
		enums.SettingsLayout:    snapshot.Layout,
		enums.SettingsSSH:       snapshot.SSH,
		enums.SettingsTerminal:  snapshot.Terminal,
		enums.SettingsDownloads: snapshot.Downloads,
		enums.SettingsStorage:   snapshot.Storage,
	}
	payloads := make(map[enums.SettingsCategory][]byte, len(values))
	for category, value := range values {
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		payloads[category], err = preserveUnknownFields(existing[category], encoded)
		if err != nil {
			return nil, err
		}
	}
	return payloads, nil
}

func preserveUnknownFields(existing, updated []byte) ([]byte, error) {
	var existingFields map[string]json.RawMessage
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &existingFields); err != nil {
			return nil, err
		}
	}
	var updatedFields map[string]json.RawMessage
	if err := json.Unmarshal(updated, &updatedFields); err != nil {
		return nil, err
	}
	for key, value := range existingFields {
		if _, replaced := updatedFields[key]; !replaced {
			updatedFields[key] = value
		}
	}
	return json.Marshal(updatedFields)
}

func applyDefaultCategory(snapshot *model.SettingsSnapshot, defaults model.SettingsSnapshot, category enums.SettingsCategory) {
	switch category {
	case enums.SettingsGeneral:
		snapshot.General = defaults.General
	case enums.SettingsTheme:
		snapshot.Theme = defaults.Theme
	case enums.SettingsPaths:
		snapshot.Paths = defaults.Paths
	case enums.SettingsMirrors:
		snapshot.Mirrors = defaults.Mirrors
	case enums.SettingsLogging:
		snapshot.Logging = defaults.Logging
	case enums.SettingsMonitoring:
		snapshot.Monitoring = defaults.Monitoring
	case enums.SettingsFirewall:
		snapshot.Firewall = defaults.Firewall
	case enums.SettingsLayout:
		snapshot.Layout = defaults.Layout
	case enums.SettingsSSH:
		snapshot.SSH = defaults.SSH
	case enums.SettingsTerminal:
		snapshot.Terminal = defaults.Terminal
	case enums.SettingsDownloads:
		snapshot.Downloads = defaults.Downloads
	case enums.SettingsStorage:
		snapshot.Storage = defaults.Storage
	}
}

func clonePayloads(payloads map[enums.SettingsCategory][]byte) map[enums.SettingsCategory][]byte {
	cloned := make(map[enums.SettingsCategory][]byte, len(payloads))
	for category, payload := range payloads {
		cloned[category] = append([]byte(nil), payload...)
	}
	return cloned
}

func copySubscribers(subscribers map[uint64]Subscriber) []Subscriber {
	result := make([]Subscriber, 0, len(subscribers))
	for _, subscriber := range subscribers {
		result = append(result, subscriber)
	}
	return result
}
