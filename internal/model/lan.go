package model

import (
	"errors"
	"net"
	"net/netip"
)

// ValidateLAN is shared by the control plane and clients. Unknown versions fail closed.
func ValidateLAN(g Game) error {
	n := g.Network
	if n.Version != LANVersion || len(n.EthernetTypes) > 16 {
		return errors.New("unsupported LAN version or too many Ethernet types")
	}
	seen := map[uint16]bool{}
	for _, typ := range n.EthernetTypes {
		if seen[typ] || (typ != 0 && typ < 1536) || typ == 0x0800 || typ == 0x0806 || typ == 0x86dd || typ == 0x8100 || typ == 0x88a8 {
			return errors.New("invalid additional Ethernet type")
		}
		seen[typ] = true
	}
	return ValidateGamePorts(g.Ports)
}

// Port intervals are never expanded into per-port database rows or firewall
// rules. The full TCP/UDP port space is supported, bounded only by HTTP size.
func ValidateGamePorts(ports []GamePort) error {
	var seen [2][65536]bool
	for _, p := range ports {
		end := PortEnd(p)
		if (p.Protocol != "tcp" && p.Protocol != "udp") || p.Port == 0 || end < p.Port || (p.Description != "" && !ValidLabel(p.Description, 120)) {
			return errors.New("invalid LAN TCP/UDP port range")
		}
		proto := 0
		if p.Protocol == "udp" {
			proto = 1
		}
		for port := int(p.Port); port <= int(end); port++ {
			if seen[proto][port] {
				return errors.New("overlapping game port ranges")
			}
			seen[proto][port] = true
		}
	}
	return nil
}

func PortEnd(p GamePort) uint16 {
	if p.PortEnd != 0 {
		return p.PortEnd
	}
	return p.Port
}

func ParseLANMAC(s string) (net.HardwareAddr, error) {
	m, err := net.ParseMAC(s)
	if err != nil || len(m) != 6 || m[0]&1 != 0 || s == "00:00:00:00:00:00" {
		return nil, errors.New("LAN MAC must be a nonzero unicast Ethernet address")
	}
	zero := true
	for _, b := range m {
		zero = zero && b == 0
	}
	if zero {
		return nil, errors.New("zero LAN MAC")
	}
	return m, nil
}

// The room is an isolated link. Addresses are derived from the already unique,
// quarantined certificate IPv4 allocation; no second allocator is needed.
func LANIPv6(ip netip.Addr) netip.Addr {
	v := ip.As4()
	b := [16]byte{0xfd, 0x4e, 0x4c, 0x52}
	copy(b[12:], v[:])
	return netip.AddrFrom16(b)
}

func LANLinkLocal(mac net.HardwareAddr) netip.Addr {
	b := [16]byte{0xfe, 0x80}
	b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15] = mac[0]^2, mac[1], mac[2], 0xff, 0xfe, mac[3], mac[4], mac[5]
	return netip.AddrFrom16(b)
}
