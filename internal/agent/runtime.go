package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	nodeMode        bool
	nodeServer      string
	nodeState       NodeState
	nodeSelected    *model.Node
	nodePending     string
	nodeLastRenewal time.Time
	nodeControlOK   bool
	udpTraffic      *udpTraffic
	telemetryAt     time.Time
	telemetryOffset int
	renewAt         time.Time
	logs            *LogBuffer

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
	probeBusy        atomic.Bool
	engineGeneration uint64
	registered       map[string]time.Time
	imageSlots       chan struct{}
	wake             chan struct{}
	watchRoom        string
	watchCancel      context.CancelFunc
	relays           []string
	relaySelected    time.Time
}

func New(dir string, log *slog.Logger) (*Runtime, error) {
	r := &Runtime{dir: dir, log: log, engine: engine.New(log), imageSlots: make(chan struct{}, 2), registered: map[string]time.Time{}, wake: make(chan struct{}, 1), status: model.Status{Control: "unconfigured", Engine: "stopped", Peers: []model.Peer{}}}
	i, err := platform.LoadIdentity(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err = device.ValidateURL(i.Server); err != nil {
			return nil, err
		}
		r.identity = i
		r.api = client.NewAPI(i)
		r.setPublicIdentity(i)
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
		r.status.Error = err.Error()
	} else {
		r.status.Error = ""
	}
	r.stateMu.Unlock()
	if err != nil {
		r.log.Warn("agent state", "control", control, "error", err)
	}
}

func (r *Runtime) persist(i device.Identity) error {
	if err := platform.SaveIdentity(r.dir, i); err != nil {
		return err
	}
	r.identity = i
	r.setPublicIdentity(i)
	return nil
}

func (r *Runtime) setPublicIdentity(i device.Identity) {
	r.stateMu.Lock()
	r.status.DeviceID = i.ID()
	r.status.Server, r.status.Name, r.status.SelectedRoom = i.Server, i.Name, i.RoomID
	r.status.Ports = append([]model.EndpointRequest{}, i.Ports...)
	r.stateMu.Unlock()
}

func (r *Runtime) Run(ctx context.Context) error {
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
				r.netMu.Lock()
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
					op.Error = err.Error()
					if len(op.Error) > 2000 {
						op.Error = op.Error[:2000]
					}
					r.nodeState.Operations[id] = op
					changed = true
				}
			}
			if changed {
				if e := r.saveNodeState(); e != nil {
					r.log.Warn("operation result persistence failed", "error", e)
				}
			}
		}
		connection := "unreachable"
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
