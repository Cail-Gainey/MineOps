package service

import (
	"sort"
	"strings"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// UnsavedItem identifies one frontend editor or form currently holding uncommitted work.
type UnsavedItem struct {
	Owner string `json:"owner"`
	Label string `json:"label"`
}

// ExitGuard coordinates synchronous native close interception with frontend dirty-state reporting.
type ExitGuard struct {
	mu        sync.Mutex
	items     map[string]string
	forceNext bool
}

// NewExitGuard creates the application-wide unsaved content registry.
func NewExitGuard() *ExitGuard {
	return &ExitGuard{items: make(map[string]string)}
}

// SetDirty registers or clears one bounded editor/form dirty state.
func (g *ExitGuard) SetDirty(owner, label string, dirty bool) error {
	owner, label = strings.TrimSpace(owner), strings.TrimSpace(label)
	if owner == "" || len(owner) > 160 || len(label) > 240 || strings.ContainsAny(owner+label, "\x00\r\n") {
		return apperror.New(apperror.CodeValidationInvalidArgument, "未保存内容标识或说明无效")
	}
	g.mu.Lock()
	if dirty {
		if label == "" {
			label = owner
		}
		g.items[owner] = label
	} else {
		delete(g.items, owner)
	}
	g.mu.Unlock()
	return nil
}

// RequestQuit returns false with a stable dirty-item snapshot unless one confirmed force request is pending.
func (g *ExitGuard) RequestQuit() (bool, []UnsavedItem) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.forceNext {
		g.forceNext = false
		return true, nil
	}
	items := make([]UnsavedItem, 0, len(g.items))
	for owner, label := range g.items {
		items = append(items, UnsavedItem{Owner: owner, Label: label})
	}
	sort.Slice(items, func(left, right int) bool { return items[left].Owner < items[right].Owner })
	return len(items) == 0, items
}

// ConfirmQuit authorizes exactly one native quit request without silently discarding registry state early.
func (g *ExitGuard) ConfirmQuit() {
	g.mu.Lock()
	g.forceNext = true
	g.mu.Unlock()
}

// Items returns the current stable unsaved-content snapshot.
func (g *ExitGuard) Items() []UnsavedItem {
	g.mu.Lock()
	items := make([]UnsavedItem, 0, len(g.items))
	for owner, label := range g.items {
		items = append(items, UnsavedItem{Owner: owner, Label: label})
	}
	g.mu.Unlock()
	sort.Slice(items, func(left, right int) bool { return items[left].Owner < items[right].Owner })
	return items
}
