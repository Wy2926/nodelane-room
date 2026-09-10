package agent

import (
	"encoding/binary"
	"sync/atomic"

	"github.com/nodelane/nodelane-room/internal/model"
	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

// Passive, header-only UDP accounting includes relay forwarding, which never
// crosses the TUN. No packets are retained, modified, or injected.
type udpTraffic struct {
	fd     int
	port   int
	stop   chan struct{}
	done   chan struct{}
	tx, rx atomic.Uint64
	failed atomic.Bool
}

func startUDPTraffic(port int) *udpTraffic {
	if port < 1 || port > 65535 {
		return nil
	}
	// ETH_P_ALL receives outgoing taps too; filter to IPv4 before inspecting headers.
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		return nil
	}
	code, err := bpf.Assemble([]bpf.Instruction{
		bpf.LoadExtension{Num: bpf.ExtProto}, bpf.JumpIf{Cond: bpf.JumpEqual, Val: unix.ETH_P_IP, SkipFalse: 10},
		bpf.LoadAbsolute{Off: 9, Size: 1}, bpf.JumpIf{Cond: bpf.JumpEqual, Val: 17, SkipFalse: 8},
		bpf.LoadAbsolute{Off: 6, Size: 2}, bpf.JumpIf{Cond: bpf.JumpBitsSet, Val: 0x3fff, SkipTrue: 6},
		bpf.LoadMemShift{Off: 0}, bpf.LoadIndirect{Off: 0, Size: 2},
		bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(port), SkipTrue: 2},
		bpf.LoadIndirect{Off: 2, Size: 2}, bpf.JumpIf{Cond: bpf.JumpEqual, Val: uint32(port), SkipFalse: 1},
		bpf.RetConstant{Val: 68}, bpf.RetConstant{Val: 0},
	})
	if err != nil {
		unix.Close(fd)
		return nil
	}
	filter := make([]unix.SockFilter, len(code))
	for i, op := range code {
		filter[i] = unix.SockFilter{Code: op.Op, Jt: op.Jt, Jf: op.Jf, K: op.K}
	}
	if unix.SetsockoptSockFprog(fd, unix.SOL_SOCKET, unix.SO_ATTACH_FILTER, &unix.SockFprog{Len: uint16(len(filter)), Filter: &filter[0]}) != nil {
		unix.Close(fd)
		return nil
	}
	u := &udpTraffic{fd: fd, port: port, stop: make(chan struct{}), done: make(chan struct{})}
	go u.read()
	return u
}

func htons(n uint16) uint16 {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], n)
	return binary.NativeEndian.Uint16(b[:])
}

func (u *udpTraffic) read() {
	defer close(u.done)
	defer unix.Close(u.fd)
	buf := make([]byte, 68)
	for {
		select {
		case <-u.stop:
			return
		default:
		}
		if _, err := unix.Poll([]unix.PollFd{{Fd: int32(u.fd), Events: unix.POLLIN}}, 250); err != nil && err != unix.EINTR {
			u.failed.Store(true)
			return
		}
		n, from, err := unix.Recvfrom(u.fd, buf, unix.MSG_DONTWAIT)
		if err == unix.EAGAIN || err == unix.EINTR {
			continue
		}
		if err != nil {
			u.failed.Store(true)
			return
		}
		a, ok := from.(*unix.SockaddrLinklayer)
		if !ok || n < 28 {
			continue
		}
		h := int(buf[0]&15) * 4
		if h < 20 || n < h+8 {
			continue
		}
		length := binary.BigEndian.Uint16(buf[h+4 : h+6])
		if length < 8 {
			continue
		}
		if a.Pkttype == unix.PACKET_OUTGOING && int(binary.BigEndian.Uint16(buf[h:h+2])) == u.port {
			u.tx.Add(uint64(length - 8))
		} else if a.Pkttype == unix.PACKET_HOST && int(binary.BigEndian.Uint16(buf[h+2:h+4])) == u.port {
			u.rx.Add(uint64(length - 8))
		}
	}
}

func (u *udpTraffic) Close() {
	if u != nil {
		close(u.stop)
		<-u.done
	}
}
func (u *udpTraffic) Sample() *model.TrafficSample {
	if u == nil || u.failed.Load() {
		return nil
	}
	stats, err := unix.GetsockoptTpacketStats(u.fd, unix.SOL_PACKET, unix.PACKET_STATISTICS)
	if err != nil || stats.Drops > 0 {
		u.failed.Store(true)
		return nil
	}
	return &model.TrafficSample{UploadBytes: u.tx.Load(), DownloadBytes: u.rx.Load(), Scope: "nebula_udp"}
}
