package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/nodelane/nodelane-room/internal/client"
	"github.com/nodelane/nodelane-room/internal/model"
	"github.com/nodelane/nodelane-room/internal/platform"
)

type NodeState struct {
	Desired          *model.Node                    `json:"desired,omitempty"`
	Applied          *model.NodeConfig              `json:"applied,omitempty"`
	AppliedRevision  int64                          `json:"applied_revision"`
	ApprovedRevision int64                          `json:"approved_revision"`
	ListenPort       int                            `json:"listen_port"`
	Operations       map[string]model.NodeOperation `json:"operations"`
}

func (r *Runtime) ConfigureNode(server string, port int) error {
	r.nodeMode = true
	if port < 0 || port > 65535 || (port > 0 && port < 1024) {
		return errors.New("UDP port must be 1024–65535")
	}
	if server != "" {
		if err := client.ValidateURL(server); err != nil {
			return err
		}
	}
	if r.api != nil {
		if !r.identity.Node {
			return errors.New("state directory contains a player identity")
		}
		if server != "" && r.identity.Server != server {
			return errors.New("identity belongs to another control server")
		}
		server = r.identity.Server
	}
	if server == "" {
		return errors.New("configure the control HTTPS address with --server")
	}
	r.nodeServer = server
	b, err := os.ReadFile(filepath.Join(r.dir, "node-state.json"))
	if err == nil {
		if err = json.Unmarshal(b, &r.nodeState); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if r.nodeState.Operations == nil {
		r.nodeState.Operations = map[string]model.NodeOperation{}
	}
	if port != 0 {
		r.nodeState.ListenPort = port
	}
	if r.identity.NodeID == "" {
		r.setError("awaiting_enrollment", nil)
	}
	return nil
}
func (r *Runtime) saveNodeState() error {
	b, err := json.Marshal(r.nodeState)
	if err != nil {
		return err
	}
	if err = platform.SecureDir(r.dir); err != nil {
		return err
	}
	f, err := os.CreateTemp(r.dir, ".node-state-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = os.Rename(name, filepath.Join(r.dir, "node-state.json")); err != nil {
		return err
	}
	return platform.SyncDir(r.dir)
}
func (r *Runtime) EnrollNode(ctx context.Context, key string) (model.NodeLocalStatus, error) {
	r.op.Lock()
	defer r.op.Unlock()
	if !r.nodeMode {
		return model.NodeLocalStatus{}, errors.New("not a node service")
	}
	i := r.identity
	if i.ID() == "" {
		var err error
		i, err = client.NewIdentity(r.nodeServer, "infrastructure")
		if err != nil {
			return model.NodeLocalStatus{}, err
		}
		i.Node = true
		if err = r.persist(i); err != nil {
			return model.NodeLocalStatus{}, err
		}
	}
	api := client.NewAPI(i)
	// Authentication recovers a registration committed before the final local write.
	if err := api.Authenticate(ctx); err != nil {
		if i.NodeID != "" {
			return r.nodeStatusLocked(), err
		}
		if len(key) != 64 {
			return r.nodeStatusLocked(), errors.New("enter the 64-character temporary enrollment key")
		}
		if err = api.Enroll(ctx, key); err != nil {
			return r.nodeStatusLocked(), err
		}
	}
	var sync model.NodeSync
	if err := api.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Report: model.NodeReport{Version: model.Version, Engine: "stopped"}}, &sync); err != nil {
		return r.nodeStatusLocked(), err
	}
	i.NodeID = sync.Node.ID
	i.Generation = sync.Node.Generation
	if err := r.persist(i); err != nil {
		return r.nodeStatusLocked(), err
	}
	r.api = api
	n := sync.Node
	n.Report = model.NodeReport{}
	r.nodeState.Desired = &n
	if err := r.saveNodeState(); err != nil {
		return r.nodeStatusLocked(), err
	}
	r.Wake()
	return r.nodeStatusLocked(), nil
}
func (r *Runtime) nodeReport() model.NodeReport {
	s := r.Status()
	return model.NodeReport{Version: model.Version, Engine: s.Engine, Error: s.Error, AppliedRevision: r.nodeState.AppliedRevision, AppliedConfig: r.nodeState.Applied, LeaseExpiresAt: s.LeaseExpiresAt, LastRenewal: r.nodeLastRenewal, Probes: r.nodeProbes()}
}
func (r *Runtime) nodeStatusLocked() model.NodeLocalStatus {
	return model.NodeLocalStatus{Status: r.Status(), Node: r.nodeState.Desired, Report: r.nodeReport(), Server: r.nodeServer, Registered: r.identity.NodeID != "", Version: model.Version}
}
func (r *Runtime) NodeStatus() model.NodeLocalStatus {
	r.op.Lock()
	defer r.op.Unlock()
	return r.nodeStatusLocked()
}
func (r *Runtime) ApplyNodeConfig(revision int64) error {
	r.op.Lock()
	defer r.op.Unlock()
	n := r.nodeState.Desired
	if n == nil || n.Revision != revision {
		return errors.New("configuration version changed; run nlroom-node config again")
	}
	_, p, err := net.SplitHostPort(n.Address)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return err
	}
	if os.Getenv("NLROOM_DEPLOYMENT") == "container" && os.Getenv("NLROOM_MAPPED_PORT") != p {
		return fmt.Errorf("update Compose UDP mapping and NLROOM_MAPPED_PORT to %s, recreate the container, then apply configuration", p)
	}
	r.nodeState.ApprovedRevision = revision
	r.nodeState.ListenPort = port
	if err = r.saveNodeState(); err != nil {
		return err
	}
	r.Wake()
	return nil
}
func (r *Runtime) RestartNode() error {
	r.op.Lock()
	defer r.op.Unlock()
	if !r.nodeMode {
		return errors.New("not a node service")
	}
	r.netMu.Lock()
	r.stopNetworkLocked()
	r.netMu.Unlock()
	r.Wake()
	return nil
}
func (r *Runtime) prepareNode(ctx context.Context) (model.Snapshot, bool, error) {
	var out model.NodeSync
	results := []model.OperationResult{}
	for _, op := range r.nodeState.Operations {
		if time.Now().Before(op.ExpiresAt) {
			results = append(results, model.OperationResult{ID: op.ID, State: op.State, Error: op.Error})
			if len(results) == 64 {
				break
			}
		}
	}
	if err := r.api.Call(ctx, "POST", "/v2/node/sync", model.NodeSyncRequest{Generation: r.identity.Generation, Report: r.nodeReport(), Results: results}, &out); err != nil {
		if client.IsDenied(err) {
			r.netMu.Lock()
			r.stopNetworkLocked()
			r.netMu.Unlock()
		}
		return out.Snapshot, false, err
	}
	if delta := time.Since(out.Snapshot.ServerTime); delta > 30*time.Second || delta < -30*time.Second {
		return out.Snapshot, false, errors.New("system clock differs from control server by more than 30 seconds")
	}
	r.nodeControlOK = true
	n := out.Node
	if r.identity.Generation != 0 && (r.identity.Generation != n.Generation || r.identity.NodeID != n.ID) {
		return out.Snapshot, false, errors.New("node identity generation changed")
	}
	changed := r.nodeState.Desired == nil || r.nodeState.Desired.Revision != n.Revision || r.nodeState.Desired.State != n.State
	// The committed response acknowledges terminal results. Keep running checkpoints
	// until completed, and keep unsubmitted results for the next bounded batch.
	for _, result := range results {
		if result.State == "succeeded" || result.State == "failed" {
			delete(r.nodeState.Operations, result.ID)
			changed = true
		}
	}
	n.Report = model.NodeReport{}
	r.nodeState.Desired = &n
	r.nodePending = ""
	selected := n
	_, portText, err := net.SplitHostPort(n.Address)
	if err != nil {
		return out.Snapshot, false, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return out.Snapshot, false, err
	}
	if r.nodeState.ListenPort == 0 {
		r.nodeState.ListenPort = port
		changed = true
	}
	portApproval := false
	if old := r.nodeState.Applied; old != nil {
		_, oldPort, _ := net.SplitHostPort(old.Address)
		portApproval = oldPort != portText && r.nodeState.ApprovedRevision != n.Revision
	}
	if r.nodeState.ListenPort != port || portApproval {
		r.nodePending = fmt.Sprintf("configuration v%d awaits local UDP port %d; run nlroom-node config apply %d after updating the deployment", n.Revision, port, n.Revision)
		if old := r.nodeState.Applied; old != nil {
			selected.Name = old.Name
			selected.Region = old.Region
			selected.Address = old.Address
			selected.Lighthouse = old.Lighthouse
			selected.Relay = old.Relay
			selected.Notes = old.Notes
			selected.Revision = r.nodeState.AppliedRevision
		} else {
			if err = r.saveNodeState(); err != nil {
				return out.Snapshot, false, err
			}
			return out.Snapshot, false, errors.New(r.nodePending)
		}
	}
	r.nodeSelected = &selected
	for id, op := range r.nodeState.Operations {
		if time.Now().After(op.ExpiresAt) {
			delete(r.nodeState.Operations, id)
			changed = true
		}
	}
	for _, op := range out.Operations {
		if op.Generation != n.Generation || op.ExpiresAt.Before(time.Now()) || op.Revision > n.Revision {
			continue
		}
		if _, seen := r.nodeState.Operations[op.ID]; seen {
			continue
		}
		op.State = "running"
		r.nodeState.Operations[op.ID] = op
		// Persist the execution checkpoint before a restart. If the process crashes,
		// its next start already performs that restart and only needs to acknowledge.
		if err = r.saveNodeState(); err != nil {
			return out.Snapshot, false, err
		}
		if op.Action == "restart" {
			r.netMu.Lock()
			r.stopNetworkLocked()
			r.netMu.Unlock()
		}
	}
	if changed {
		if err = r.saveNodeState(); err != nil {
			return out.Snapshot, false, err
		}
	}
	if n.State == "disabled" {
		r.netMu.Lock()
		r.stopNetworkLocked()
		r.netMu.Unlock()
		if err = r.nodeApplied(); err != nil {
			return out.Snapshot, false, err
		}
		r.setError("connected", nil)
		return out.Snapshot, false, nil
	}
	if n.State != "active" && n.State != "draining" {
		return out.Snapshot, false, errors.New("node is not authorized to run")
	}
	return out.Snapshot, true, nil
}
func (r *Runtime) nodeApplied() error {
	if r.nodeSelected == nil {
		return nil
	}
	n := r.nodeSelected
	c := n.Config()
	changed := r.nodeState.AppliedRevision != n.Revision || r.nodeState.Applied == nil
	r.nodeState.Applied = &c
	r.nodeState.AppliedRevision = n.Revision
	for id, op := range r.nodeState.Operations {
		if op.State == "running" && op.Revision <= n.Revision {
			op.State = "succeeded"
			r.nodeState.Operations[id] = op
			changed = true
		}
	}
	if changed {
		return r.saveNodeState()
	}
	return nil
}
func (r *Runtime) nodeProbes() []model.NodeProbe {
	r.netMu.Lock()
	out := []model.NodeProbe{}
	if r.probe == nil {
		r.netMu.Unlock()
		return out
	}
	for _, n := range r.snapshot.Nodes {
		if n.DeviceID == r.identity.ID() {
			continue
		}
		at := r.probe.LastSample(n.IP)
		if at.IsZero() {
			continue
		}
		mode, remote := r.engine.Path(n.IP)
		rtt, loss := r.probe.Stats(n.IP)
		// A failed latest probe cannot be presented as current connectivity.
		if !r.probe.LastSuccess(n.IP) {
			mode = "unreachable"
		}
		out = append(out, model.NodeProbe{DeviceID: n.DeviceID, Address: n.Address, Remote: remote, Mode: mode, RTTMillis: rtt, LossPercent: loss, At: at})
		if len(out) == 64 {
			break
		}
	}
	r.netMu.Unlock()
	// DNS never holds the data-plane lock: certificate expiry must remain independent.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := range out {
		if out[i].Mode == "direct" && publicRemote(ctx, out[i].Address, out[i].Remote) {
			out[i].Mode = "direct-public"
		}
	}
	return out
}
func publicRemote(ctx context.Context, address, remote string) bool {
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	rh, rp, err := net.SplitHostPort(remote)
	if err != nil || p != rp {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.Equal(net.ParseIP(rh))
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return false
	}
	for _, candidate := range ips {
		if candidate.IP.Equal(net.ParseIP(rh)) {
			return true
		}
	}
	return false
}

func (r *Runtime) nodeRevision() int64 {
	if r.nodeSelected != nil {
		return r.nodeSelected.Revision
	}
	return 0
}
