package client

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type Identity struct {
	Server        string                  `json:"server"`
	Name          string                  `json:"name"`
	PrivateKey    []byte                  `json:"private_key"`
	RoomID        string                  `json:"room_id,omitempty"`
	NodeID        string                  `json:"node_id,omitempty"`
	Generation    int64                   `json:"generation,omitempty"`
	Node          bool                    `json:"node"`
	Ports         []model.EndpointRequest `json:"ports,omitempty"`
	CAFingerprint string                  `json:"ca_fingerprint,omitempty"`
}

func NewIdentity(server, name string) (Identity, error) {
	if err := ValidateURL(server); err != nil {
		return Identity{}, err
	}
	if !model.ValidLabel(name, 80) {
		return Identity{}, errors.New("name must contain 1-80 bytes")
	}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	return Identity{Server: strings.TrimRight(server, "/"), Name: name, PrivateKey: key}, err
}
func ValidateURL(server string) error {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
		return errors.New("server must be an HTTPS origin")
	}
	if u.Scheme == "https" {
		return nil
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil
	}
	return errors.New("HTTP is allowed only on loopback; use HTTPS for remote servers")
}
func (i Identity) ID() string {
	if len(i.PrivateKey) != ed25519.PrivateKeySize {
		return ""
	}
	p := ed25519.PrivateKey(i.PrivateKey).Public().(ed25519.PublicKey)
	h := sha256.Sum256(p)
	return hex.EncodeToString(h[:])
}
func ID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("control API %d (%s): %s", e.Status, e.Code, e.Message)
}
func IsDenied(err error) bool {
	var e *APIError
	return errors.As(err, &e) && (e.Status == 403 || e.Status == 404)
}

type API struct {
	Identity Identity
	HTTP     *http.Client
	mu       sync.Mutex
	session  model.Session
}

func NewAPI(i Identity) *API {
	return &API{Identity: i, HTTP: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (a *API) Authenticate(ctx context.Context) error { _, err := a.login(ctx); return err }
func (a *API) login(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.session.Token != "" && time.Until(a.session.ExpiresAt) > time.Minute {
		return a.session.Token, nil
	}
	if len(a.Identity.PrivateKey) != ed25519.PrivateKeySize {
		return "", errors.New("invalid device identity")
	}
	pub := ed25519.PrivateKey(a.Identity.PrivateKey).Public().(ed25519.PublicKey)
	scope, prefix := "player", "/v2/auth"
	if a.Identity.Node {
		scope, prefix = "node", "/v2/node/auth"
	}
	var challenge model.Challenge
	if err := a.request(ctx, "POST", prefix+"/challenge", model.ChallengeRequest{DeviceID: a.Identity.ID(), Name: a.Identity.Name, PublicKey: pub}, &challenge, "", ID()); err != nil {
		return "", err
	}
	message := append([]byte("nodelane-auth-v2:"+scope+":"+challenge.ID+":"), challenge.Nonce...)
	sig := ed25519.Sign(a.Identity.PrivateKey, message)
	if err := a.request(ctx, "POST", prefix+"/verify", model.VerifyRequest{ID: challenge.ID, Signature: sig}, &a.session, "", ID()); err != nil {
		return "", err
	}
	return a.session.Token, nil
}
func (a *API) Call(ctx context.Context, method, path string, in, out any) error {
	key := ID()
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		token, e := a.login(ctx)
		if e != nil {
			return e
		}
		err = a.request(ctx, method, path, in, out, token, key)
		if err == nil {
			return nil
		}
		var apierr *APIError
		if errors.As(err, &apierr) {
			if apierr.Status == 401 {
				a.mu.Lock()
				if a.session.Token == token {
					a.session = model.Session{}
				}
				a.mu.Unlock()
				continue
			}
			if apierr.Status < 500 {
				return err
			}
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}
func (a *API) request(ctx context.Context, method, path string, in, out any, token, key string) error {
	var body []byte
	var err error
	if in != nil {
		body, err = json.Marshal(in)
		if err != nil {
			return err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, a.Identity.Server+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Idempotency-Key", key)
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := a.HTTP.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := &APIError{Status: resp.StatusCode, Code: "http_error", Message: http.StatusText(resp.StatusCode)}
		_ = json.Unmarshal(b, e)
		return e
	}
	if out != nil {
		return json.Unmarshal(b, out)
	}
	return nil
}
func (a *API) Watch(ctx context.Context, room string, after int64, onSnapshot func(model.Snapshot)) error {
	token, err := a.login(ctx)
	if err != nil {
		return err
	}
	r, err := http.NewRequestWithContext(ctx, "GET", a.Identity.Server+"/v2/rooms/"+room+"/events", nil)
	if err != nil {
		return err
	}
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Last-Event-ID", fmt.Sprint(after))
	h := *a.HTTP
	h.Timeout = 0
	resp, err := h.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &APIError{Status: resp.StatusCode, Message: "event stream unavailable"}
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	kind := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			kind = strings.TrimPrefix(line, "event: ")
		}
		if kind == "snapshot" && strings.HasPrefix(line, "data: ") {
			var s model.Snapshot
			if err = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &s); err != nil {
				return err
			}
			onSnapshot(s)
		}
	}
	if err = scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}
