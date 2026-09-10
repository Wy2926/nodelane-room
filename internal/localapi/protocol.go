package localapi

import "encoding/json"

type Request struct {
	Action string          `json:"action"`
	Room   string          `json:"room,omitempty"`
	Server string          `json:"server,omitempty"`
	Name   string          `json:"name,omitempty"`
	Target string          `json:"target,omitempty"`
	Body   json.RawMessage `json:"body,omitempty"`
}

type LogEntry struct {
	Sequence uint64 `json:"sequence"`
	Line     string `json:"line"`
}
