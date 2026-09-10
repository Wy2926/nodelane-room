package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"os"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/engine"
	"github.com/nodelane/nodelane-room/internal/game"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/pki"
	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/nodelane/nodelane-room/internal/probe"
	"github.com/slackhq/nebula/cert"
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
	identity         client.Identity
	api              *client.API
	status           model.Status
	engine           *engine.Engine
	lease            model.Lease
	key, public      []byte
	snapshot         model.Snapshot
	probe            *probe.Service
	game             *game.Minecraft
	gameView         atomic.Pointer[game.Minecraft]
	selfID           string
	probeBusy        atomic.Bool
	engineGeneration uint64
	registered       map[string]time.Time
	wake             chan struct{}
	watchRoom        string
	watchCancel      context.CancelFunc
	relays           []string
	relaySelected    time.Time
}

func New(dir string, log *slog.Logger) (*Runtime, error) {
	r := &Runtime{dir: dir, log: log, engine: engine.New(log), registered: map[string]time.Time{}, wake: make(chan struct{}, 1), status: model.Status{Control: "unconfigured", Engine: "stopped", Peers: []model.Peer{}}}
	i, err := platform.LoadIdentity(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err = client.ValidateURL(i.Server); err != nil {
			return nil, err
		}
		r.identity = i
		r.api = client.NewAPI(i)
		r.status.DeviceID = i.ID()
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
func (r *Runtime) persist(i client.Identity) error {
	if err := platform.SaveIdentity(r.dir, i); err != nil {
		return err
	}
	r.identity = i
	r.stateMu.Lock()
	r.status.DeviceID = i.ID()
	r.stateMu.Unlock()
	return nil
}
func (r *Runtime) Init(ctx context.Context, server, name string) (string, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api != nil {
		return "", errors.New("device is already initialized")
	}
	i, err := client.NewIdentity(server, name)
	if err != nil {
		return "", err
	}
	a := client.NewAPI(i)
	if err = a.Authenticate(ctx); err != nil {
		return "", err
	}
	if err = r.persist(i); err != nil {
		return "", err
	}
	r.api = a
	r.Wake()
	return i.ID(), nil
}
func (r *Runtime) Action(ctx context.Context, action, room string, body json.RawMessage) (any, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if r.api == nil {
		return nil, errors.New("run nodelane init first")
	}
	if r.identity.Node {
		return nil, errors.New("infrastructure identity cannot operate player rooms")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	path := "/v2/rooms/" + room + "/" + action
	if action == "create" {
		path = "/v2/rooms"
	}
	if action == "join" {
		path = "/v2/rooms/join"
	}
	if room == "" && action != "create" && action != "join" {
		return nil, errors.New("no selected room")
	}
	var out json.RawMessage
	if err := r.api.Call(ctx, "POST", path, body, &out); err != nil {
		return nil, err
	}
	i := r.identity
	if action == "create" || action == "join" {
		var result model.RoomResult
		if err := json.Unmarshal(out, &result); err != nil {
			return nil, err
		}
		i.RoomID = result.Room.ID
		i.Ports = nil
		r.registered = map[string]time.Time{}
		if err := r.persist(i); err != nil {
			return nil, err
		}
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
	}
	if (action == "leave" || action == "close") && room == i.RoomID {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		i.RoomID = ""
		i.Ports = nil
		if err := r.persist(i); err != nil {
			return nil, err
		}
	}
	r.Wake()
	return out, nil
}
func (r *Runtime) Members(ctx context.Context, room string) (model.Snapshot, error) {
	r.op.Lock()
	defer r.op.Unlock()
	var s model.Snapshot
	if r.api == nil {
		return s, errors.New("run init first")
	}
	if room == "" {
		room = r.identity.RoomID
	}
	if room == "" {
		return s, errors.New("no selected room")
	}
	err := r.api.Call(ctx, "GET", "/v2/rooms/"+room, nil, &s)
	return s, err
}
func (r *Runtime) AddPort(in model.EndpointRequest) error {
	r.op.Lock()
	defer r.op.Unlock()
	if r.identity.RoomID == "" {
		return errors.New("join a room first")
	}
	if (in.Protocol != "tcp" && in.Protocol != "udp") || in.Port == 0 || in.Port == model.ProbePort {
		return errors.New("invalid game port")
	}
	i := r.identity
	for _, p := range i.Ports {
		if p.Protocol == in.Protocol && p.Port == in.Port {
			return nil
		}
	}
	if len(i.Ports) >= 16 {
		return errors.New("at most 16 explicit ports are supported")
	}
	i.Ports = append(i.Ports, in)
	if err := r.persist(i); err != nil {
		return err
	}
	r.Wake()
	return nil
}
func (r *Runtime) Status() model.Status {
	r.stateMu.Lock()
	s := r.status
	r.stateMu.Unlock()
	r.netMu.Lock()
	defer r.netMu.Unlock()
	s.Room = r.snapshot.Room
	s.IP = r.lease.IP
	s.LeaseExpiresAt = r.lease.ExpiresAt
	s.Engine = "stopped"
	if r.engine.Running() {
		s.Engine = "running"
	}
	s.Peers = r.engine.Peers(r.snapshot.Members)
	if r.probe != nil {
		for i := range s.Peers {
			s.Peers[i].RTTMillis, s.Peers[i].LossPercent = r.probe.Stats(s.Peers[i].IP)
		}
	}
	return s
}
func (r *Runtime) Ping(ctx context.Context, target string) (map[string]any, error) {
	r.netMu.Lock()
	p := r.probe
	ip := ""
	matches := 0
	for _, m := range r.snapshot.Members {
		if target == m.IP || target == m.DeviceID || target == m.Name {
			ip = m.IP
			matches++
		}
	}
	r.netMu.Unlock()
	if matches > 1 {
		return nil, errors.New("nickname is ambiguous; use device ID or virtual IP")
	}
	if p == nil || ip == "" {
		return nil, errors.New("connected room member not found")
	}
	_ = r.engine.Connect(ip)
	d, err := p.Ping(ctx, ip)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ip": ip, "rtt_ms": float64(d) / float64(time.Millisecond)}, nil
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
	adsDone := make(chan struct{})
	go func() {
		defer close(adsDone)
		t := time.NewTicker(1500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.advertise()
			}
		}
	}()
	defer func() {
		if r.watchCancel != nil {
			r.watchCancel()
		}
		<-done
		<-adsDone
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
func (r *Runtime) step(ctx context.Context) error {
	if r.api == nil || (r.nodeMode && r.identity.NodeID == "") {
		return nil
	}
	i := r.identity
	if i.RoomID == "" && !i.Node {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.snapshot = model.Snapshot{}
		r.netMu.Unlock()
		if r.watchCancel != nil {
			r.watchCancel()
			r.watchCancel = nil
			r.watchRoom = ""
		}
		r.setError("idle", nil)
		return nil
	}
	if !i.Node && r.watchRoom != i.RoomID {
		if r.watchCancel != nil {
			r.watchCancel()
		}
		watchCtx, cancel := context.WithCancel(ctx)
		r.watchCancel = cancel
		r.watchRoom = i.RoomID
		api, room := r.api, i.RoomID
		go func() {
			var rev int64
			for watchCtx.Err() == nil {
				_ = api.Watch(watchCtx, room, rev, func(s model.Snapshot) {
					if s.Room != nil {
						rev = s.Room.Revision
					}
					r.Wake()
				})
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-watchCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	prefix := "/v2/rooms/" + i.RoomID
	var snapshot model.Snapshot
	if i.Node {
		prefix = "/v2/node"
		var allowed bool
		var err error
		snapshot, allowed, err = r.prepareNode(ctx)
		if err != nil {
			return err
		}
		if !allowed {
			return nil
		}
	} else {
		if err := r.api.Call(ctx, "POST", prefix+"/heartbeat", struct{}{}, nil); err != nil {
			if client.IsDenied(err) {
				r.netMu.Lock()
				r.stopNetworkLocked()
				r.netMu.Unlock()
				r.setError("connected", err)
				return nil
			}
			return err
		}
		if err := r.api.Call(ctx, "GET", prefix, nil, &snapshot); err != nil {
			return err
		}
	}
	if delta := time.Since(snapshot.ServerTime); delta > 30*time.Second || delta < -30*time.Second {
		return errors.New("system clock differs from control server by more than 30 seconds")
	}
	// Apply revocations immediately, even when the following renewal request
	// fails. A successful snapshot must not leave obsolete permissions active.
	r.netMu.Lock()
	if r.engine.Running() {
		current := engine.Config{Lease: r.lease, Snapshot: snapshot, PrivateKey: r.key, DeviceID: i.ID(), Interface: "nodelane0", RelayIPs: r.selectRelaysLocked(snapshot.Nodes)}
		if err := r.engine.Apply(current); err != nil {
			r.stopNetworkLocked()
			r.netMu.Unlock()
			return err
		}
		if r.engine.Generation() != r.engineGeneration {
			r.closeAdaptersLocked()
		}
		r.snapshot = snapshot
		if r.probe != nil {
			r.probe.Update(snapshot)
		}
		if r.game != nil {
			r.game.Update(snapshot)
		}
	}
	r.netMu.Unlock()
	r.netMu.Lock()
	lease := r.lease
	renew := lease.IP == "" || (!r.renewAt.IsZero() && !time.Now().Before(r.renewAt)) || (r.renewAt.IsZero() && time.Until(lease.ExpiresAt) < 7*time.Minute)
	key, pub := r.key, r.public
	r.netMu.Unlock()
	if renew {
		if len(key) == 0 {
			var err error
			key, pub, err = pki.TunnelKey()
			if err != nil {
				return err
			}
		}
		if err := r.api.Call(ctx, "POST", prefix+"/lease", model.LeaseRequest{PublicKey: pub, Revision: r.nodeRevision()}, &lease); err != nil {
			if client.IsDenied(err) {
				r.netMu.Lock()
				r.stopNetworkLocked()
				r.netMu.Unlock()
			}
			return err
		}
	}
	if i.Node && r.nodeSelected != nil {
		n := *r.nodeSelected
		lease.Node = &n
	}
	cfg := engine.Config{Lease: lease, Snapshot: snapshot, PrivateKey: key, DeviceID: i.ID(), Interface: "nodelane0"}
	if err := engine.Validate(cfg); err != nil {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		return err
	}
	ca, _, err := cert.UnmarshalCertificateFromPEM([]byte(lease.CA))
	if err != nil {
		return err
	}
	fp, err := ca.Fingerprint()
	if err != nil {
		return err
	}
	if i.CAFingerprint != "" && i.CAFingerprint != fp {
		return errors.New("control server changed the pinned Nebula CA")
	}
	if i.CAFingerprint == "" {
		i.CAFingerprint = fp
		if err = r.persist(i); err != nil {
			return err
		}
	}
	r.netMu.Lock()
	cfg.RelayIPs = r.selectRelaysLocked(snapshot.Nodes)
	if err = r.engine.Apply(cfg); err != nil {
		r.netMu.Unlock()
		return err
	}
	if r.engine.Generation() != r.engineGeneration {
		r.closeAdaptersLocked()
	}
	r.engineGeneration = r.engine.Generation()
	if i.Node && r.udpTraffic == nil {
		r.udpTraffic = startUDPTraffic(r.nodeState.ListenPort)
	}
	r.lease = lease
	if renew {
		r.renewAt = lease.ExpiresAt.Add(-7 * time.Minute).Add(time.Duration(rand.IntN(15)) * time.Second)
		r.nodeLastRenewal = time.Now().UTC()
	}
	r.key = key
	r.public = pub
	r.snapshot = snapshot
	r.selfID = i.ID()
	if r.probe == nil {
		r.probe, err = probe.Start(lease.IP, lease.Network, i.Node, func(ip, room, endpoint string) {
			if g := r.gameView.Load(); g != nil {
				g.Notice(ip, room, endpoint)
			}
		})
		if err != nil {
			r.stopNetworkLocked()
			r.netMu.Unlock()
			return err
		}
	}
	r.probe.Update(snapshot)
	var gameError error
	if !i.Node && snapshot.Room != nil && snapshot.Room.Game == "minecraft-java" && r.game == nil {
		r.game, gameError = game.StartMinecraft(i.ID())
		r.gameView.Store(r.game)
	}
	if r.game != nil {
		r.game.Update(snapshot)
	}
	p := r.probe
	members := append([]model.Member(nil), snapshot.Members...)
	nodes := append([]model.Node(nil), snapshot.Nodes...)
	g := r.game
	r.netMu.Unlock()
	if i.Node {
		if err = r.nodeApplied(); err != nil {
			return err
		}
	}
	r.stateMu.Lock()
	r.status.Control = "connected"
	r.status.Error = r.nodePending
	if gameError != nil {
		r.status.Error = gameError.Error()
	}
	r.stateMu.Unlock()
	if r.probeBusy.CompareAndSwap(false, true) {
		go func() {
			defer r.probeBusy.Store(false)
			var wg sync.WaitGroup
			ips := map[string]bool{}
			for _, m := range members {
				if m.DeviceID != i.ID() {
					ips[m.IP] = true
				}
			}
			for _, n := range nodes {
				if n.DeviceID != i.ID() {
					ips[n.IP] = true
				}
			}
			if i.Node {
				observed, _ := r.engine.Network()
				for _, peer := range observed.Peers {
					if len(ips) < 128 {
						ips[peer.IP] = true
					}
				}
			}
			for ip := range ips {
				wg.Add(1)
				go func(ip string) { defer wg.Done(); _ = r.engine.Connect(ip); _, _ = p.Ping(ctx, ip) }(ip)
			}
			wg.Wait()
		}()
	}
	if !i.Node {
		ports := append([]model.EndpointRequest(nil), i.Ports...)
		if g != nil {
			for _, o := range g.Observations() {
				ports = append(ports, model.EndpointRequest{Protocol: o.Protocol, Port: o.Port, MOTD: o.MOTD})
			}
		}
		for _, e := range ports {
			k := e.Protocol + ":" + strconv.Itoa(int(e.Port)) + ":" + e.MOTD
			if time.Since(r.registered[k]) < 10*time.Second {
				continue
			}
			if err = r.api.Call(ctx, "POST", prefix+"/endpoints", e, nil); err != nil {
				return err
			}
			r.registered[k] = time.Now()
		}
	}
	return nil
}
func (r *Runtime) selectRelaysLocked(nodes []model.Node) []string {
	type candidate struct {
		ip      string
		latency float64
	}
	candidates := []candidate{}
	healthy := map[string]bool{}
	for _, n := range nodes {
		if !n.Relay || n.Draining || n.DeviceID == r.identity.ID() {
			continue
		}
		latency := 1e9
		if r.probe != nil {
			rtt, _ := r.probe.Stats(n.IP)
			if rtt != nil {
				latency = *rtt
			}
			if r.probe.Failed(n.IP) {
				continue
			}
		}
		candidates = append(candidates, candidate{n.IP, latency})
		healthy[n.IP] = true
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].latency == candidates[j].latency {
			return candidates[i].ip < candidates[j].ip
		}
		return candidates[i].latency < candidates[j].latency
	})
	keep := len(r.relays) > 0
	for _, ip := range r.relays {
		if !healthy[ip] {
			keep = false
		}
	}
	if keep && time.Since(r.relaySelected) < time.Minute {
		return r.relays
	}
	if keep && len(candidates) > 0 && r.probe != nil {
		current, _ := r.probe.Stats(r.relays[0])
		if current != nil && candidates[0].latency >= *current*0.8 {
			r.relaySelected = time.Now()
			return r.relays
		}
	}
	r.relays = []string{}
	for _, c := range candidates {
		if len(r.relays) == 2 {
			break
		}
		r.relays = append(r.relays, c.ip)
	}
	r.relaySelected = time.Now()
	return r.relays
}
func (r *Runtime) advertise() {
	r.netMu.Lock()
	defer r.netMu.Unlock()
	if r.game == nil || r.probe == nil || !r.engine.Running() {
		return
	}
	local := map[uint16]bool{}
	for _, o := range r.game.Observations() {
		local[o.Port] = true
	}
	for _, e := range r.snapshot.Endpoints {
		if e.DeviceID != r.selfID || e.Protocol != "tcp" || !local[e.Port] {
			continue
		}
		for _, m := range r.snapshot.Members {
			if m.DeviceID != e.DeviceID {
				_ = r.probe.Advertise(m.IP, e.ID)
			}
		}
	}
}
func (r *Runtime) stopNetworkLocked() {
	r.engine.Stop()
	r.closeAdaptersLocked()
	r.lease = model.Lease{}
	r.key = nil
	r.public = nil
	r.relays = nil
}
func (r *Runtime) closeAdaptersLocked() {
	r.udpTraffic.Close()
	r.udpTraffic = nil
	r.gameView.Store(nil)
	if r.probe != nil {
		r.probe.Close()
		r.probe = nil
	}
	if r.game != nil {
		r.game.Close()
		r.game = nil
	}
}
func (r *Runtime) Doctor() map[string]any {
	s := r.Status()
	result := map[string]any{"nebula_version": model.NebulaVersion, "device_id": s.DeviceID, "control": s.Control, "engine": s.Engine, "error": s.Error, "platform": platform.Diagnostics(), "peers": s.Peers}
	if s.IP != "" {
		result["lease_expires_at"] = s.LeaseExpiresAt
		result["virtual_ip"] = s.IP
	}
	return result
}

func (r *Runtime) String() string { return fmt.Sprintf("NodeLane agent (%s)", r.dir) }
