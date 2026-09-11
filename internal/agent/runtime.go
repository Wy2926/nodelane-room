package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/device"
	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/nodelane/nodelane-room/internal/probe"
)

type Runtime struct {
	commandMu         sync.Mutex
	commandView       atomic.Value
	commands          map[string]model.SavedCommand
	paused            atomic.Bool
	pauseMu           sync.Mutex
	networkEpoch      atomic.Uint64
	statusSeq         atomic.Uint64
	instanceID        string
	updateMu          sync.Mutex
	updateState       model.UpdateStatus
	updateWake        chan struct{}
	guiVersion        string
	versionReportedAt time.Time
	lastVersionReport model.ClientReport
	nodeMode          bool
	nodeServer        string
	nodeState         NodeState
	nodeSelected      *model.Node
	nodePending       string
	nodeLastRenewal   time.Time
	nodeControlOK     bool
	udpTraffic        *udpTraffic
	telemetryAt       time.Time
	telemetryOffset   int
	renewAt           time.Time
	logs              *LogBuffer

	op               sync.Mutex
	stateMu          sync.Mutex
	netMu            sync.Mutex
	dir              string
	log              *slog.Logger
	identity         device.Identity
	api              *client.API
	status           model.Status
	engine           *engine.Engine
	lease            model.Lease
	key, public      []byte
	snapshot         model.Snapshot
	probe            *probe.Service
	engineGeneration uint64
	imageSlots       chan struct{}
	wake             chan struct{}
	watchRoom        string
	watchCancel      context.CancelFunc
	relays           []string
	relaySelected    time.Time
}

func New(dir string, log *slog.Logger) (*Runtime, error) {
	r := &Runtime{dir: dir, log: log, engine: engine.New(log), imageSlots: make(chan struct{}, 2), wake: make(chan struct{}, 1), status: model.Status{Control: "unconfigured", Engine: "stopped", Peers: []model.Peer{}}}
	r.instanceID = client.ID()
	if err := r.loadCommands(); err != nil {
		return nil, err
	}
	if b, e := platform.LoadPrivateFile(filepath.Join(dir, "network-paused.bin")); e == nil {
		r.paused.Store(string(b) == "true")
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, model.Failure("local_storage_failed")
	}
	i, err := platform.LoadIdentity(dir)
	r.updateWake = make(chan struct{}, 1)
	if b, e := platform.LoadPrivateFile(filepath.Join(dir, "update-policy.bin")); e == nil {
		_ = json.Unmarshal(b, &r.updateState.Policy)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, model.Failure("local_identity_unreadable")
	}
	if err == nil {
		if err = device.ValidateURL(i.Server); err != nil {
			return nil, model.Failure("local_identity_unreadable")
		}
		r.identity = i
		if !i.SignedOut {
			r.api = client.NewAPI(i)
		}
		r.setPublicIdentity(i)
		if i.SignedOut {
			r.status.Control = "signed_out"
		}
	}
	return r, nil
}

func (r *Runtime) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runtime) setError(control string, err error) {
	r.stateMu.Lock()
	r.status.Control = control
	if err != nil {
		out := serviceFailure(err, "")
		scope := "network"
		if model.EndsIdentity(out.Code) {
			scope = "account"
		} else if model.EndsMembership(out.Code) {
			scope = "room"
		} else if model.IsCode(err, "local_probe_unavailable", "local_peer_unreachable", "local_probe_target_unavailable", "local_probe_target_ambiguous") {
			scope = "peer"
		}
		r.status.Error = out.Message
		found := false
		for _, issue := range r.status.Issues {
			if issue.Code == out.Code && issue.ResolvedAt == nil {
				found = true
			}
		}
		if !found {
			r.status.Issues = append(r.status.Issues, model.Issue{Scope: scope, Code: out.Code, OccurredAt: time.Now().UTC()})
			if len(r.status.Issues) > 16 {
				r.status.Issues = r.status.Issues[len(r.status.Issues)-16:]
			}
		}
	} else {
		r.status.Error = ""
		now := time.Now().UTC()
		for i := range r.status.Issues {
			if r.status.Issues[i].ResolvedAt == nil {
				r.status.Issues[i].ResolvedAt = &now
			}
		}
	}
	r.stateMu.Unlock()
	if err != nil {
		r.log.Warn("agent state", "control", control, "code", model.Code(err))
	}
}

func (r *Runtime) persist(i device.Identity) error {
	if err := platform.SaveIdentity(r.dir, i); err != nil {
		return model.Failure("local_storage_failed")
	}
	r.identity = i
	r.setPublicIdentity(i)
	return nil
}

func (r *Runtime) setPublicIdentity(i device.Identity) {
	r.stateMu.Lock()
	r.status.DeviceID = i.ID()
	r.status.Server, r.status.Name, r.status.SelectedRoom = i.Server, i.Name, i.RoomID
	r.stateMu.Unlock()
}

func (r *Runtime) Run(ctx context.Context) error {
	probeDone := make(chan struct{})
	go func() { defer close(probeDone); r.runProbes(ctx) }()
	defer func() { <-probeDone }()
	updateDone := make(chan struct{})
	go func() {
		defer close(updateDone)
		if !r.nodeMode {
			r.runUpdates(ctx)
		}
	}()
	defer func() { <-updateDone }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		t := time.NewTicker(250 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				required := requiredLocally(r.updateStatus().Policy)
				r.netMu.Lock()
				if required {
					r.stopNetworkLocked()
				}
				r.engine.Expire(time.Now())
				if r.lease.IP != "" && !time.Now().Before(r.lease.ExpiresAt) {
					r.stopNetworkLocked()
				}
				r.netMu.Unlock()
			}
		}
	}()
	defer func() {
		if r.watchCancel != nil {
			r.watchCancel()
		}
		<-done
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	r.Wake()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-r.wake:
		}
		if ctx.Err() != nil {
			return nil
		}
		r.op.Lock()
		r.nodeControlOK = false
		err := r.step(ctx)
		if err == nil {
			r.reportTelemetry(ctx)
		}
		if r.nodeMode && err != nil && ctx.Err() == nil {
			changed := false
			for id, op := range r.nodeState.Operations {
				if op.State == "running" {
					op.State = "failed"
					op.Error = model.Code(err)
					if len(op.Error) > 2000 {
						op.Error = op.Error[:2000]
					}
					r.nodeState.Operations[id] = op
					changed = true
				}
			}
			if changed {
				if e := r.saveNodeState(); e != nil {
					r.log.Warn("operation result persistence failed", "code", "local_storage_failed")
				}
			}
		}
		r.stateMu.Lock()
		connection := r.status.Control
		r.stateMu.Unlock()
		var remote *client.APIError
		if errors.As(err, &remote) {
			connection = "connected"
		}
		if model.IsCode(err, "local_control_unreachable", "local_dns_failed", "local_control_timeout", "local_control_connection_lost", "local_tls_failed") {
			connection = "unreachable"
		}
		if r.nodeMode && r.nodeControlOK {
			connection = "connected"
		}
		r.op.Unlock()
		if err != nil && ctx.Err() == nil {
			r.setError(connection, err)
		}
	}
}

func (r *Runtime) String() string { return fmt.Sprintf("NodeLane agent (%s)", r.dir) }
