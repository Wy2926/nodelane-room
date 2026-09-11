package model

import (
	"encoding/json"
	"errors"
	"time"
)

const Contract = "interaction-1"
const ContractHeader = "X-NodeLane-Contract"
const DeadlineHeader = "X-NodeLane-Operation-Deadline"

type Retry struct {
	Kind    string `json:"kind"`
	AfterMS int64  `json:"after_ms,omitempty"`
}

type FieldIssue struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// Details contains facts only. Request bodies, errors and credentials never belong here.
type Details struct {
	Fields           []FieldIssue `json:"fields,omitempty"`
	ExpectedRevision int64        `json:"expected_revision,omitempty"`
	ActualRevision   int64        `json:"actual_revision,omitempty"`
	Capacity         int          `json:"capacity,omitempty"`
	MemberCount      int          `json:"member_count,omitempty"`
	AllowedBytes     int64        `json:"allowed_bytes,omitempty"`
	KnownCommit      bool         `json:"known_commit,omitempty"`
	RoomID           string       `json:"room_id,omitempty"`
	DeviceID         string       `json:"device_id,omitempty"`
	ExpiresAt        *time.Time   `json:"expires_at,omitempty"`
}

type Result struct {
	Contract          string          `json:"contract"`
	Code              string          `json:"code"`
	Message           string          `json:"message"`
	Origin            string          `json:"origin"`
	RequestID         string          `json:"request_id"`
	OperationID       string          `json:"operation_id,omitempty"`
	ServerTime        *time.Time      `json:"server_time,omitempty"`
	ObservedAt        *time.Time      `json:"observed_at,omitempty"`
	ControlHTTPStatus *int            `json:"control_http_status"`
	CauseRequestID    string          `json:"cause_request_id,omitempty"`
	Data              json.RawMessage `json:"data"`
	Details           Details         `json:"details"`
	Retry             Retry           `json:"retry"`
}

type BusinessError struct {
	Code    string
	Details Details
}

func (e *BusinessError) Error() string { return e.Code }

func Failure(code string) *BusinessError { return &BusinessError{Code: code} }

func Code(err error) string {
	var e *BusinessError
	if errors.As(err, &e) {
		return e.Code
	}
	return "system_internal_error"
}

func IsCode(err error, codes ...string) bool {
	for _, code := range codes {
		if Code(err) == code {
			return true
		}
	}
	return false
}

func Validation(field, rule string) error {
	return &BusinessError{Code: "request_validation_failed", Details: Details{Fields: []FieldIssue{{Field: field, Rule: rule}}}}
}

func RevisionError(expected, actual int64) error {
	return &BusinessError{Code: "request_state_stale", Details: Details{ExpectedRevision: expected, ActualRevision: actual}}
}

func EndsMembership(code string) bool {
	switch code {
	case "member_kicked", "member_taken_over", "member_left", "member_revoked", "room_closed", "room_expired", "room_banned":
		return true
	}
	return false
}

func EndsIdentity(code string) bool {
	switch code {
	case "auth_device_revoked", "auth_device_expired", "auth_device_unregistered", "account_deleted", "auth_identity_scope_conflict":
		return true
	}
	return false
}

func NewResult(code, origin, requestID string, data any) Result {
	spec, ok := BusinessCodes[code]
	if !ok {
		code = "system_internal_error"
		spec = BusinessCodes[code]
	}
	r := Result{Contract: Contract, Code: code, Message: spec.Message, Origin: origin, RequestID: requestID, Retry: Retry{Kind: spec.Retry}}
	if data != nil {
		if b, err := json.Marshal(data); err == nil {
			r.Data = b
		} else {
			return NewResult("system_internal_error", origin, requestID, nil)
		}
	}
	return r
}

type CodeSpec struct {
	Status  int
	Message string
	Retry   string
}

func HTTPStatus(code string) int {
	if spec, ok := BusinessCodes[code]; ok {
		return spec.Status
	}
	return 500
}

// SafeDetails enforces the public field set at both HTTP and IPC boundaries.
func SafeDetails(code string, in Details) Details {
	out := Details{}
	switch code {
	case "request_validation_failed":
		for _, issue := range in.Fields {
			field := false
			switch issue.Field {
			case "server", "name", "room", "room_id", "device_id", "network", "ports", "capacity", "game", "expected_revision", "expected_game_revision", "operation_deadline", "username", "password", "database_url", "public_url", "registry", "ca_mode", "revision", "reason":
				field = true
			}
			rule := false
			switch issue.Rule {
			case "required", "blank", "too_long", "out_of_range", "invalid_format", "unsupported_value", "unknown_field", "overlap", "policy_missing":
				rule = true
			}
			if field && rule {
				out.Fields = append(out.Fields, issue)
			}
		}
	case "request_state_stale":
		out.ExpectedRevision = in.ExpectedRevision
		out.ActualRevision = in.ActualRevision
	case "account_in_use", "room_already_joined":
		out.RoomID = in.RoomID
		out.DeviceID = in.DeviceID
		out.ActualRevision = in.ActualRevision
	case "room_full":
		out.Capacity = in.Capacity
		out.MemberCount = in.MemberCount
	case "request_too_large":
		out.AllowedBytes = in.AllowedBytes
	case "operation_expired", "auth_device_expired", "room_expired":
		out.ExpiresAt = in.ExpiresAt
	}
	// This fact describes a confirmed prior write, including a failed local repair.
	out.KnownCommit = in.KnownCommit
	return out
}
