package lan

import (
	"bytes"
	"net/netip"
	"time"
)

type fragmentKey struct {
	src, dst netip.Addr
	target   netip.Addr
	id       uint32
	proto    byte
}
type fragmentPart struct {
	offset      int
	data, frame []byte
}
type fragmentSet struct {
	expires     time.Time
	total, size int
	parts       []fragmentPart
}

// Reassemble before applying port rules. Non-first fragments never receive an
// unconditional permit and overlaps cannot replace an already checked header.
func (d *Device) defragment(p packet, frame []byte, target netip.Addr, now time.Time) (packet, [][]byte, bool) {
	if !p.fragmented {
		return p, [][]byte{frame}, true
	}
	k := fragmentKey{p.src, p.dst, target, p.id, p.proto}
	for key, s := range d.ipFragments {
		if !now.Before(s.expires) {
			delete(d.ipFragments, key)
		}
	}
	s := d.ipFragments[k]
	if s == nil {
		if len(d.ipFragments) >= 64 {
			return p, nil, false
		}
		s = &fragmentSet{expires: now.Add(2 * time.Second), total: -1}
		d.ipFragments[k] = s
	}
	end := p.offset + len(p.data)
	if len(s.parts) >= 128 || s.size+len(p.data) > 65535 || s.total >= 0 && (end > s.total || !p.more && end != s.total) {
		delete(d.ipFragments, k)
		return p, nil, false
	}
	for _, q := range s.parts {
		if p.offset < q.offset+len(q.data) && q.offset < end {
			if p.offset == q.offset && bytes.Equal(p.data, q.data) {
				return p, nil, false
			}
			delete(d.ipFragments, k)
			return p, nil, false
		}
	}
	if !p.more {
		for _, q := range s.parts {
			if q.offset+len(q.data) > end {
				delete(d.ipFragments, k)
				return p, nil, false
			}
		}
		s.total = end
	}
	s.parts = append(s.parts, fragmentPart{p.offset, bytes.Clone(p.data), bytes.Clone(frame)})
	s.size += len(p.data)
	if s.total < 0 || s.size != s.total {
		return p, nil, false
	}
	p.data = make([]byte, s.total)
	frames := make([][]byte, 0, len(s.parts))
	for _, q := range s.parts {
		copy(p.data[q.offset:], q.data)
		frames = append(frames, q.frame)
	}
	p.fragmented = false
	p.offset = 0
	p.more = false
	delete(d.ipFragments, k)
	return p, frames, true
}
