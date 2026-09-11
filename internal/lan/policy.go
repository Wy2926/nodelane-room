package lan

import (
	"bytes"
	"encoding/binary"
	"net"
	"net/netip"
	"time"

	"github.com/nodelane/nodelane-room/internal/model"
)

type member struct {
	ip      netip.Addr
	mac     net.HardwareAddr
	expires time.Time
}
type flow struct {
	src, dst         netip.Addr
	srcPort, dstPort uint16
	proto            byte
}

func (m member) owns(ip netip.Addr) bool {
	return ip == m.ip || ip == model.LANIPv6(m.ip) || ip == model.LANLinkLocal(m.mac)
}

func (d *Device) active(m member, now time.Time) bool {
	return m.ip.IsValid() && len(m.mac) == 6 && now.Before(m.expires) && now.Before(d.expires)
}

func (d *Device) permitted(frame []byte, source, target member, now time.Time) [][]byte {
	if len(frame) < 14 || len(frame) > maxFrame || !d.active(source, now) || !d.active(target, now) || !bytes.Equal(frame[6:12], source.mac) {
		return nil
	}
	broadcast := bytes.Equal(frame[:6], []byte{255, 255, 255, 255, 255, 255})
	multicast := frame[0]&1 != 0 && !broadcast
	if !broadcast && !multicast && !bytes.Equal(frame[:6], target.mac) {
		return nil
	}
	typ := binary.BigEndian.Uint16(frame[12:14])
	if typ == 0x0806 {
		b := frame[14:]
		if len(b) < 28 || binary.BigEndian.Uint16(b[:2]) != 1 || binary.BigEndian.Uint16(b[2:4]) != 0x0800 || b[4] != 6 || b[5] != 4 || !bytes.Equal(b[8:14], source.mac) {
			return nil
		}
		op := binary.BigEndian.Uint16(b[6:8])
		src := netip.AddrFrom4([4]byte(b[14:18]))
		dst := netip.AddrFrom4([4]byte(b[24:28]))
		if (op != 1 && op != 2) || src != source.ip && !(src.IsUnspecified() && op == 1) || dst != target.ip && dst != source.ip {
			return nil
		}
		return [][]byte{frame}
	}
	if !d.enabled {
		return nil
	}
	if typ != 0x0800 && typ != 0x86dd {
		if broadcast && !d.broadcast || multicast && !d.multicast {
			return nil
		}
		if typ <= 1500 {
			if int(typ) > len(frame)-14 || typ == 0 {
				return nil
			}
			// LLC/SNAP cannot tunnel IP, ARP or VLAN past the IP/MAC checks.
			if len(frame) >= 22 && bytes.Equal(frame[14:20], []byte{0xaa, 0xaa, 3, 0, 0, 0}) {
				snap := binary.BigEndian.Uint16(frame[20:22])
				if snap == 0x0800 || snap == 0x0806 || snap == 0x86dd || snap == 0x8100 || snap == 0x88a8 {
					return nil
				}
			}
			typ = 0
		}
		if d.etherTypes[typ] {
			return [][]byte{frame}
		}
		return nil
	}
	p, ok := parseIP(frame[14:])
	if !ok || (typ == 0x0800) != p.src.Is4() {
		return nil
	}
	isND := p.proto == 58 && !p.fragmented && len(p.data) >= 24 && (p.data[0] == 135 || p.data[0] == 136)
	if !source.owns(p.src) && !(isND && p.data[0] == 135 && p.src.IsUnspecified()) {
		return nil
	}
	group := p.dst.IsMulticast() || p.dst == netip.MustParseAddr("255.255.255.255") || p.dst == d.broadcastIP
	if group {
		if !broadcast && !multicast {
			return nil
		}
		if !isND && (broadcast && !d.broadcast || multicast && !d.multicast) {
			return nil
		}
	} else if !target.owns(p.dst) || broadcast || multicast {
		return nil
	}
	// Neighbour discovery is link control, never an advertisement of a gateway.
	if isND {
		if frame[21] != 255 || p.data[1] != 0 {
			return nil
		}
		targetIP := netip.AddrFrom16([16]byte(p.data[8:24]))
		if p.data[0] == 136 && !source.owns(targetIP) || p.data[0] == 135 && !target.owns(targetIP) && !source.owns(targetIP) {
			return nil
		}
		for b := p.data[24:]; len(b) > 0; {
			if len(b) < 2 || b[1] == 0 || int(b[1])*8 > len(b) {
				return nil
			}
			n := int(b[1]) * 8
			if (b[0] == 1 || b[0] == 2) && (n != 8 || !bytes.Equal(b[2:8], source.mac)) {
				return nil
			}
			b = b[n:]
		}
		return [][]byte{frame}
	}
	if p.proto == 58 && !p.fragmented && len(p.data) >= 8 && ((p.data[0] >= 1 && p.data[0] <= 4) || p.data[0] == 128 || p.data[0] == 129) {
		return [][]byte{frame}
	}
	if p.proto == 1 && !p.fragmented && validICMP(p.data) {
		return [][]byte{frame}
	}
	// IGMP/MLD membership is local to the TAP; sender replication does not need
	// to expose queriers, routers, DHCP or physical-network control traffic.
	if p.proto != 6 && p.proto != 17 {
		return nil
	}
	p, frames, ok := d.defragment(p, frame, target.ip, now)
	if !ok {
		return nil
	}
	if len(p.data) < 8 {
		return nil
	}
	if p.proto == 17 && int(binary.BigEndian.Uint16(p.data[4:6])) != len(p.data) {
		return nil
	}
	if p.proto == 6 && (len(p.data) < 20 || int(p.data[12]>>4)*4 < 20 || int(p.data[12]>>4)*4 > len(p.data)) {
		return nil
	}
	srcPort, dstPort := binary.BigEndian.Uint16(p.data[:2]), binary.BigEndian.Uint16(p.data[2:4])
	f := flow{source.ip, target.ip, srcPort, dstPort, p.proto}
	protoIndex := 0
	if p.proto == 17 {
		protoIndex = 1
	}
	if !d.ports[protoIndex][dstPort] && !now.Before(d.flows[f]) {
		return nil
	}
	if !now.Before(d.flowSweep) {
		for key, expires := range d.flows {
			if !now.Before(expires) {
				delete(d.flows, key)
			}
		}
		d.flowSweep = now.Add(time.Second)
	}
	reverse := flow{f.dst, f.src, f.dstPort, f.srcPort, f.proto}
	if _, exists := d.flows[reverse]; exists || len(d.flows) < 8192 {
		ttl := 30 * time.Second
		if p.proto == 6 {
			ttl = 5 * time.Minute
		}
		d.flows[reverse] = now.Add(ttl)
	}
	return frames
}
