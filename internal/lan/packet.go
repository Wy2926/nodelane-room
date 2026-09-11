// Package lan carries room Ethernet frames through authenticated Nebula IPv4
// packets. It owns no keys, public sockets, routes to physical LANs or NAT logic.
package lan

import (
	"encoding/binary"
	"net"
	"net/netip"
)

const maxFrame = 1514

func validICMP(b []byte) bool {
	if len(b) < 8 {
		return false
	}
	switch b[0] {
	case 0, 3, 8, 11, 12:
		return true
	}
	return false
}

type packet struct {
	src, dst         netip.Addr
	proto            byte
	data             []byte
	id               uint32
	offset           int
	more, fragmented bool
}

func parseIP(b []byte) (p packet, ok bool) {
	if len(b) < 20 {
		return p, false
	}
	switch b[0] >> 4 {
	case 4:
		h, total := int(b[0]&15)*4, int(binary.BigEndian.Uint16(b[2:4]))
		if h < 20 || h > len(b) || total < h || total > len(b) || checksum(b[:h]) != 0 {
			return p, false
		}
		p.src = netip.AddrFrom4([4]byte(b[12:16]))
		p.dst = netip.AddrFrom4([4]byte(b[16:20]))
		p.proto, p.data = b[9], b[h:total]
		frag := binary.BigEndian.Uint16(b[6:8])
		p.offset, p.more = int(frag&8191)*8, frag&0x2000 != 0
		p.fragmented = p.offset != 0 || p.more
		p.id = uint32(binary.BigEndian.Uint16(b[4:6]))
		if frag&0x8000 != 0 || p.fragmented && frag&0x4000 != 0 {
			return p, false
		}
	case 6:
		if len(b) < 40 {
			return p, false
		}
		total := 40 + int(binary.BigEndian.Uint16(b[4:6]))
		if total > len(b) || total == 40 {
			return p, false
		}
		p.src = netip.AddrFrom16([16]byte(b[8:24]))
		p.dst = netip.AddrFrom16([16]byte(b[24:40]))
		p.proto, p.data = b[6], b[40:total]
		for i := 0; i < 8; i++ {
			if p.proto == 44 {
				if len(p.data) < 8 || p.data[1] != 0 {
					return p, false
				}
				f := binary.BigEndian.Uint16(p.data[2:4])
				if f&6 != 0 {
					return p, false
				}
				p.offset, p.more, p.fragmented = int(f&0xfff8), f&1 != 0, true
				p.id = binary.BigEndian.Uint32(p.data[4:8])
				p.proto, p.data = p.data[0], p.data[8:]
				break
			}
			if p.proto != 0 && p.proto != 60 {
				break
			}
			if len(p.data) < 8 {
				return p, false
			}
			n := (int(p.data[1]) + 1) * 8
			if n > len(p.data) {
				return p, false
			}
			p.proto, p.data = p.data[0], p.data[n:]
		}
	default:
		return p, false
	}
	if p.fragmented && (len(p.data) == 0 || p.offset+len(p.data) > 65535 || p.more && len(p.data)%8 != 0) {
		return p, false
	}
	return p, true
}

func checksum(b []byte) uint16 {
	var n uint32
	for len(b) > 1 {
		n += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) > 0 {
		n += uint32(b[0]) << 8
	}
	for n>>16 != 0 {
		n = n&65535 + n>>16
	}
	return ^uint16(n)
}

func udpPacket(src, dst netip.Addr, port uint16, data []byte) []byte {
	b := make([]byte, 28+len(data))
	b[0], b[8], b[9] = 0x45, 64, 17
	binary.BigEndian.PutUint16(b[2:4], uint16(len(b)))
	copy(b[12:16], src.AsSlice())
	copy(b[16:20], dst.AsSlice())
	binary.BigEndian.PutUint16(b[10:12], checksum(b[:20]))
	binary.BigEndian.PutUint16(b[20:22], port)
	binary.BigEndian.PutUint16(b[22:24], port)
	binary.BigEndian.PutUint16(b[24:26], uint16(8+len(data)))
	copy(b[28:], data)
	return b
}

func ethernet(src, dst net.HardwareAddr, typ uint16, payload []byte) []byte {
	b := make([]byte, 14+len(payload))
	copy(b[:6], dst)
	copy(b[6:12], src)
	binary.BigEndian.PutUint16(b[12:14], typ)
	copy(b[14:], payload)
	return b
}
