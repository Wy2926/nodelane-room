package localapi

import "encoding/json"

const ProtocolVersion = 1

type Error struct {
	Code    string `json:"code"`
	Message string `json:"error"`
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func Failure(code, message string) error { return &Error{Code: code, Message: message} }

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
