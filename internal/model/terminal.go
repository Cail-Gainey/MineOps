package model

import "time"

// TerminalSessionState 标识一个远端 SSH PTY 的生命周期。
type TerminalSessionState string

const (
	TerminalOpening TerminalSessionState = "opening"
	TerminalOpen    TerminalSessionState = "open"
	TerminalClosed  TerminalSessionState = "closed"
	TerminalFailed  TerminalSessionState = "failed"
)

// TerminalSession 承载一个 SSH PTY 的身份、尺寸、状态与时间戳。
type TerminalSession struct {
	ID           ID                   `json:"id"`
	SSHSessionID ID                   `json:"sshSessionID"`
	State        TerminalSessionState `json:"state"`
	Columns      int                  `json:"columns"`
	Rows         int                  `json:"rows"`
	OpenedAt     time.Time            `json:"openedAt"`
	ClosedAt     *time.Time           `json:"closedAt,omitempty"`
}
