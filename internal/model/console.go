package model

import "time"

// ConsoleSession is a desktop attachment to one running remote Minecraft tmux session.
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
