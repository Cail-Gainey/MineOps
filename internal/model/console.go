package model

import "time"

// ConsoleSession 表示桌面对一个运行中远端 Minecraft tmux 会话的附着。
type ConsoleSession struct {
	ID                ID         `json:"id"`
	ServerID          ID         `json:"serverID"`
	ProcessIdentityID ID         `json:"processIdentityID"`
	Offset            int64      `json:"offset"`
	State             string     `json:"state"`
	ReadOnly          bool       `json:"readOnly"`
	OpenedAt          time.Time  `json:"openedAt"`
	ClosedAt          *time.Time `json:"closedAt,omitempty"`
}
