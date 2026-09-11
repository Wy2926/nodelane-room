package localapi

import (
	"encoding/json"
	"github.com/nodelane/nodelane-room/internal/model"
)

const ProtocolVersion = 3

type Error struct {
	Code    string        `json:"code"`
	Message string        `json:"message"`
	Result  *model.Result `json:"result,omitempty"`
}

func (e *Error) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error           { return model.Failure(e.Code) }
func Failure(code, message string) error { return &Error{Code: code, Message: message} }

type Request struct {
	Contract  string          `json:"contract"`
	CommandID string          `json:"command_id,omitempty"`
	Action    string          `json:"action"`
	Room      string          `json:"room,omitempty"`
	Server    string          `json:"server,omitempty"`
	Name      string          `json:"name,omitempty"`
	Target    string          `json:"target,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
}

type LogEntry struct {
	Sequence uint64 `json:"sequence"`
	Line     string `json:"line"`
}
