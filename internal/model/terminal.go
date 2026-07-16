package model

import "time"

// TerminalSessionState identifies the lifecycle of one remote SSH PTY.
type TerminalSessionState string

const (
	TerminalOpening TerminalSessionState = "opening"
	TerminalOpen    TerminalSessionState = "open"
	TerminalClosed  TerminalSessionState = "closed"
	TerminalFailed  TerminalSessionState = "failed"
)

// TerminalSession contains one SSH PTY identity, dimensions, state, and timestamps.
type TerminalSession struct {
	ID           ID                   `json:"id"`
	SSHSessionID ID                   `json:"sshSessionID"`
	State        TerminalSessionState `json:"state"`
	Columns      int                  `json:"columns"`
	Rows         int                  `json:"rows"`
	OpenedAt     time.Time            `json:"openedAt"`
	ClosedAt     *time.Time           `json:"closedAt,omitempty"`
}
