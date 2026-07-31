package service

import (
	"sort"
	"strings"
	"sync"

	"github.com/Cail-Gainey/MineOps/internal/global/apperror"
)

// UnsavedItem 标识一个当前持有未提交内容的前端编辑器或表单。
type UnsavedItem struct {
	Owner string `json:"owner"`
	Label string `json:"label"`
}

// ExitGuard 把原生关闭的同步拦截与前端未保存状态上报协调起来。
type ExitGuard struct {
	mu        sync.Mutex
	items     map[string]string
	forceNext bool
}

// NewExitGuard 创建应用级的未保存内容登记表。
func NewExitGuard() *ExitGuard {
	return &ExitGuard{items: make(map[string]string)}
}

// SetDirty 登记或清除一个有界的编辑器或表单未保存状态。
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

// RequestQuit 返回 false 并附带稳定的未保存项快照,除非已有一次确认过的强制退出请求。
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

// ConfirmQuit 恰好放行一次原生退出请求,且不提前静默丢弃登记表状态。
func (g *ExitGuard) ConfirmQuit() {
	g.mu.Lock()
	g.forceNext = true
	g.mu.Unlock()
}

// Items 返回当前稳定的未保存内容快照。
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
