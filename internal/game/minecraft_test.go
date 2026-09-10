package game

import (
	"github.com/nodelane/nodelane-room/internal/model"
	"io"
	"net"
	"strconv"
	"testing"
	"time"
)

func TestMinecraftAnnouncement(t *testing.T) {
	motd, port, err := ParseAnnouncement(Announcement("朋友的世界", 51234))
	if err != nil || motd != "朋友的世界" || port != 51234 {
		t.Fatalf("%s %d %v", motd, port, err)
	}
	for _, s := range []string{"", "[MOTD]x[/MOTD][AD]0[/AD]", "[MOTD]x[/MOTD][AD]65536[/AD]", "[MOTD]x[/MOTD][AD]192.168.1.1:5[/AD]", "[MOTD]x\ny[/MOTD][AD]5[/AD]", "[MOTD]x[/MOTD][AD]5[/AD]suffix"} {
		if _, _, err := ParseAnnouncement([]byte(s)); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestDiscoveryRequiresAuthorizedSourceAndExpires(t *testing.T) {
	m := &Minecraft{device: "self", localIP: "127.0.0.1", remotes: map[string]*remote{}, events: make(chan DiscoveryEvent, 16)}
	s := model.Snapshot{Room: &model.Room{ID: "room"}, Members: []model.Member{{DeviceID: "host", IP: "10.203.0.3"}}, Endpoints: []model.Endpoint{{ID: "world-one", DeviceID: "host", Protocol: "tcp", Port: 25565, MOTD: "Same world name", ExpiresAt: time.Now().Add(time.Minute)}}}
	m.Update(s)
	for _, v := range [][3]string{{"10.203.0.4", "room", "world-one"}, {"10.203.0.3", "other", "world-one"}, {"10.203.0.3", "room", "fake"}} {
		m.Notice(v[0], v[1], v[2])
	}
	if len(m.remotes) != 0 {
		t.Fatal("forged discovery created proxy")
	}
	m.Notice("10.203.0.3", "room", "world-one")
	if len(m.remotes) != 1 {
		t.Fatal("authorized discovery did not create proxy")
	}
	first := m.remotes["world-one"].proxy
	defer first.Close()
	m.Notice("10.203.0.3", "room", "world-one")
	if m.remotes["world-one"].proxy != first {
		t.Fatal("duplicate notice created another proxy")
	}
	s.Endpoints[0].ExpiresAt = time.Now().Add(-time.Second)
	m.Update(s)
	if len(m.remotes) != 0 {
		t.Fatal("expired discovery retained proxy")
	}
	m.Notice("10.203.0.3", "room", "world-one")
	if len(m.remotes) != 0 {
		t.Fatal("expired endpoint recreated proxy")
	}
	m.closed = true
	s.Endpoints[0].ExpiresAt = time.Now().Add(time.Minute)
	m.Notice("10.203.0.3", "room", "world-one")
	if len(m.remotes) != 0 {
		t.Fatal("late callback recreated proxy after shutdown")
	}
}
func FuzzAnnouncement(f *testing.F) {
	f.Add([]byte("[MOTD]world[/MOTD][AD]25565[/AD]"))
	f.Fuzz(func(t *testing.T, b []byte) {
		m, p, err := ParseAnnouncement(b)
		if err == nil {
			m2, p2, e := ParseAnnouncement(Announcement(m, p))
			if e != nil || m2 != m || p2 != p {
				t.Fatal("round trip changed announcement")
			}
		}
	})
}
func TestProxyForwardsAndClosesExistingConnections(t *testing.T) {
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	remoteDone := make(chan struct{})
	go func() {
		defer close(remoteDone)
		c, e := l.Accept()
		if e == nil {
			defer c.Close()
			_, _ = io.Copy(c, c)
		}
	}()
	p, err := NewProxy("127.0.0.1", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	c, err := net.Dial("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(p.Port()))))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err = c.Write([]byte("game payload")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 12)
	if _, err = io.ReadFull(c, b); err != nil {
		t.Fatal(err)
	}
	if string(b) != "game payload" {
		t.Fatal("proxy corrupted payload")
	}
	p.Close()
	if _, err = c.Read(b); err == nil {
		t.Fatal("active connection survived proxy removal")
	}
	select {
	case <-remoteDone:
	case <-time.After(time.Second):
		t.Fatal("upstream connection leaked")
	}
}
