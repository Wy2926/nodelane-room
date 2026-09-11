package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/localapi"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

func commandWrites(action string) bool {
	switch action {
	case "create", "join", "owner-join", "invite", "invite-revoke", "kick", "transfer", "leave", "close", "account-takeover", "account-logout", "revoke-device":
		return true
	}
	return false
}

func commandPath(in localapi.Request) string {
	switch in.Action {
	case "create":
		return "/v2/rooms"
	case "join":
		return "/v2/rooms/join"
	case "owner-join":
		return "/v2/rooms/" + in.Room + "/join"
	case "invite-revoke":
		return "/v2/rooms/" + in.Room + "/invite/revoke"
	case "account-takeover":
		return "/v2/me/takeover"
	case "account-logout":
		return "/v2/me/logout"
	case "revoke-device":
		return "/v2/me/devices/" + in.Target + "/revoke"
	}
	return "/v2/rooms/" + in.Room + "/" + in.Action
}

func (r *Runtime) saveCommands() error {
	r.publishCommands()
	b, err := json.Marshal(r.commands)
	if err != nil {
		return model.Failure("local_storage_failed")
	}
	if err = platform.SavePrivateFile(filepath.Join(r.dir, "operations.bin"), b, true); err != nil {
		return model.Failure("local_storage_failed")
	}
	return nil
}

func (r *Runtime) loadCommands() error {
	r.commands = map[string]model.SavedCommand{}
	b, err := platform.LoadPrivateFile(filepath.Join(r.dir, "operations.bin"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return model.Failure("local_storage_failed")
	}
	if json.Unmarshal(b, &r.commands) != nil {
		return model.Failure("local_storage_failed")
	}
	for id, c := range r.commands {
		if !c.Deadline.After(time.Now()) && (c.State == "submitting" || c.State == "pending" || c.State == "reconciling") {
			c.State = "unresolved"
			c.Request = nil
			c.Body = nil
			c.Result = nil
		} else if c.State == "submitting" {
			c.State = "reconciling"
		}
		r.commands[id] = c
	}
	r.publishCommands()
	return nil
}

func (r *Runtime) runCommand(ctx context.Context, in localapi.Request) (any, error) {
	if !validLocalID(in.CommandID, false) || len(in.CommandID) < 16 {
		return nil, model.Failure("request_idempotency_required")
	}
	if !r.commandMu.TryLock() {
		return nil, model.Failure("local_busy")
	}
	defer r.commandMu.Unlock()
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, model.Failure("request_malformed")
	}
	digest := sha256.Sum256(append(raw, in.Body...))
	fingerprint := hex.EncodeToString(digest[:])
	if previous, ok := r.commands[in.CommandID]; ok {
		if previous.RequestHash != fingerprint {
			return nil, model.Failure("request_idempotency_conflict")
		}
		op, err := r.reconcileCommand(ctx, previous)
		if err != nil {
			return nil, err
		}
		if op.State == "unresolved" {
			return nil, model.Failure("operation_expired")
		}
		if op.State == "succeeded" && op.Result == nil {
			return model.NewResult("operation_result_redacted", "service", client.ID(), nil), nil
		}
		if op.Result != nil {
			if op.State == "rejected" {
				return nil, &model.BusinessError{Code: op.Result.Code, Details: op.Result.Details}
			}
			return op.Result.Data, nil
		}
		return model.NewResult("operation_pending", "service", client.ID(), op), nil
	}
	for _, c := range r.commands {
		if c.State == "submitting" || c.State == "pending" || c.State == "reconciling" {
			return nil, model.Failure("local_busy")
		}
	}
	for id, c := range r.commands {
		if time.Since(c.Deadline) > time.Hour {
			delete(r.commands, id)
		}
	}
	r.stateMu.Lock()
	device := r.status.DeviceID
	if in.Room == "" {
		in.Room = r.status.SelectedRoom
	}
	r.stateMu.Unlock()
	raw, err = json.Marshal(in)
	if err != nil {
		return nil, model.Failure("request_malformed")
	}
	c := model.SavedCommand{Operation: model.Operation{ID: in.CommandID, State: "submitting", Action: in.Action, Deadline: time.Now().UTC().Add(time.Hour)}, Request: raw, RequestHash: fingerprint, DeviceID: device, Method: "POST", Path: commandPath(in), Body: in.Body}
	r.commands[c.ID] = c
	if err = r.saveCommands(); err != nil {
		delete(r.commands, c.ID)
		r.publishCommands()
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	defer cancel()
	ctx = client.WithOperation(ctx, c.ID, c.Deadline)
	out, err := r.dispatch(ctx, in)
	if err == nil {
		c.State = "succeeded"
		c.KnownCommit = true
		c.Request = nil
		c.Body = nil
	} else {
		code := model.Code(err)
		status := model.HTTPStatus(code)
		c.State = "reconciling"
		if _, known := model.BusinessCodes[code]; known && status < 500 && !strings.HasPrefix(code, "local_") && code != "operation_not_found" && code != "request_rate_limited" {
			c.State = "rejected"
			c.Request = nil
			c.Body = nil
		}
		if model.IsCode(err, "local_unconfigured", "local_no_room", "local_busy", "local_already_initialized") {
			c.State = "rejected"
			c.Request = nil
			c.Body = nil
		}
		result := serviceFailure(err, c.ID)
		c.Result = &result
		c.KnownCommit = result.Details.KnownCommit
	}
	r.commands[c.ID] = c
	if saveErr := r.saveCommands(); saveErr != nil {
		c.State = "reconciling"
		r.commands[c.ID] = c
		r.publishCommands()
		return nil, &model.BusinessError{Code: "local_storage_failed", Details: model.Details{KnownCommit: c.KnownCommit}}
	}
	return out, err
}

func (r *Runtime) reconcileCommand(ctx context.Context, c model.SavedCommand) (model.Operation, error) {
	if c.Action == "account-logout" && c.State == "succeeded" {
		return c.Operation, nil
	}
	if !c.Deadline.After(time.Now()) && (c.State == "succeeded" || c.State == "rejected") {
		return c.Operation, nil
	}
	if !c.Deadline.After(time.Now()) {
		c.State = "unresolved"
		c.Request = nil
		c.Body = nil
		c.Result = nil
		r.commands[c.ID] = c
		if err := r.saveCommands(); err != nil {
			return c.Operation, err
		}
		return c.Operation, nil
	}
	r.op.Lock()
	api, device := r.api, r.identity.ID()
	r.op.Unlock()
	if api == nil || device != c.DeviceID {
		return c.Operation, model.Failure("local_unconfigured")
	}
	var result model.Operation
	if err := api.Call(ctx, "GET", "/v2/me/operations/"+c.ID, nil, &result); err != nil {
		if ((model.IsCode(err, "operation_not_found") && !c.KnownCommit) || (c.Action == "account-logout" && model.IsCode(err, "auth_device_revoked"))) && len(c.Request) > 0 {
			var in localapi.Request
			if json.Unmarshal(c.Request, &in) != nil {
				return c.Operation, model.Failure("local_storage_failed")
			}
			in.Body = append(json.RawMessage(nil), c.Body...)
			replay, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
			defer cancel()
			out, replayErr := r.dispatch(client.WithOperation(replay, c.ID, c.Deadline), in)
			if replayErr != nil {
				failure := serviceFailure(replayErr, c.ID)
				c.Result = &failure
				c.KnownCommit = failure.Details.KnownCommit
				code := model.Code(replayErr)
				if _, known := model.BusinessCodes[code]; known && model.HTTPStatus(code) < 500 && !strings.HasPrefix(code, "local_") && code != "request_rate_limited" {
					c.State = "rejected"
					c.Request = nil
					c.Body = nil
				}
				r.commands[c.ID] = c
				if e := r.saveCommands(); e != nil {
					return c.Operation, e
				}
				if c.State == "rejected" {
					return c.Operation, nil
				}
				return c.Operation, replayErr
			}
			value, ok := out.(model.Result)
			if !ok {
				value = model.NewResult("ok", "service", client.ID(), out)
			}
			result = model.Operation{ID: c.ID, State: "succeeded", Deadline: c.Deadline, KnownCommit: true, Result: &value}
		} else if model.IsCode(err, "operation_expired") {
			c.State = "unresolved"
			c.Request = nil
			c.Body = nil
			c.Result = nil
			r.commands[c.ID] = c
			return c.Operation, r.saveCommands()
		} else {
			return c.Operation, err
		}
	}
	r.op.Lock()
	defer r.op.Unlock()
	if (r.api != api && !(c.Action == "account-logout" && r.identity.SignedOut)) || r.identity.ID() != c.DeviceID {
		return c.Operation, model.Failure("local_unconfigured")
	}
	if result.KnownCommit && result.Result != nil {
		if err := r.applyActionResult(ctx, c.Action, c.Path, result.Result.Data); err != nil {
			return c.Operation, err
		}
	}
	c.Operation = result
	c.Action = r.commands[c.ID].Action
	c.Request = nil
	c.Body = nil
	stored := c
	if stored.Result != nil {
		copy := *stored.Result
		copy.Data = nil
		stored.Result = &copy
	}
	r.commands[c.ID] = stored
	if err := r.saveCommands(); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Runtime) getOperation(ctx context.Context, id string) (model.Operation, error) {
	if !r.commandMu.TryLock() {
		return model.Operation{ID: id, State: "submitting"}, nil
	}
	defer r.commandMu.Unlock()
	c, ok := r.commands[id]
	if !ok {
		return model.Operation{}, model.Failure("operation_not_found")
	}
	return r.reconcileCommand(ctx, c)
}

func (r *Runtime) pendingOperations() []model.Operation {
	if out, ok := r.commandView.Load().([]model.Operation); ok {
		return append([]model.Operation{}, out...)
	}
	return []model.Operation{}
}
func (r *Runtime) publishCommands() {
	out := []model.Operation{}
	for _, c := range r.commands {
		if c.State == "submitting" || c.State == "pending" || c.State == "reconciling" || c.State == "unresolved" {
			op := c.Operation
			op.Result = nil
			out = append(out, op)
		}
	}
	r.commandView.Store(out)
}
