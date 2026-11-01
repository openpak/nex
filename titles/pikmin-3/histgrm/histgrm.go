// Package histgrm builds the Pikmin 3 ranking histogram that the game downloads over
// SpotPass (BOSS task "histgrm") and uploads it to nn-boss.
//
// THE FILE LAYOUT HERE IS PROVISIONAL. No public source documents it: Pretendo lists the task
// (nn-boss list-known-boss-apps) but has no sample, and the SpotPass Archival Project dumped no
// Wii U data at all. Build writes a plain, versioned layout of our own so the pipeline,
// scheduling and BOSS wiring can be proven end to end; Job refuses to upload it until an
// operator sets HISTGRM_ACK_UNVERIFIED_FORMAT=1 or the layout has been checked against the
// game (see docs/histgrm.md for how).
package histgrm

import (
	"encoding/binary"
	"errors"
	"sort"
)

// Version of the provisional layout.
const Version = 1

// Buckets per category.
const Buckets = 32

// Category is one category's score distribution.
type Category struct {
	ID      uint32
	Total   uint32
	Min     uint32
	Max     uint32
	Buckets []uint32
}

// Distribute buckets scores into Buckets equal-width bins across [min, max].
func Distribute(id uint32, scores []uint32) Category {
	c := Category{ID: id, Total: uint32(len(scores)), Buckets: make([]uint32, Buckets)}
	if len(scores) == 0 {
		return c
	}
	c.Min, c.Max = scores[0], scores[0]
	for _, s := range scores {
		if s < c.Min {
			c.Min = s
		}
		if s > c.Max {
			c.Max = s
		}
	}
	span := uint64(c.Max-c.Min) + 1
	for _, s := range scores {
		i := uint64(s-c.Min) * Buckets / span
		c.Buckets[i]++
	}
	return c
}

// Build serialises categories big-endian (the Wii U's byte order), sorted by category id:
// u32 version, u32 count, then per category u32 id, total, min, max, bucket count, u32 buckets.
func Build(cats []Category) []byte {
	sorted := append([]Category(nil), cats...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	out := binary.BigEndian.AppendUint32(nil, Version)
	out = binary.BigEndian.AppendUint32(out, uint32(len(sorted)))
	for _, c := range sorted {
		for _, v := range []uint32{c.ID, c.Total, c.Min, c.Max, uint32(len(c.Buckets))} {
			out = binary.BigEndian.AppendUint32(out, v)
		}
		for _, b := range c.Buckets {
			out = binary.BigEndian.AppendUint32(out, b)
		}
	}
	return out
}

// Parse reads Build's output.
func Parse(b []byte) ([]Category, error) {
	rd := func() (uint32, error) {
		if len(b) < 4 {
			return 0, errors.New("histgrm: truncated")
		}
		v := binary.BigEndian.Uint32(b)
		b = b[4:]
		return v, nil
	}
	ver, err := rd()
	if err != nil || ver != Version {
		return nil, errors.New("histgrm: bad version")
	}
	n, err := rd()
	if err != nil {
		return nil, err
	}
	var out []Category
	for i := uint32(0); i < n; i++ {
		var h [5]uint32
		for j := range h {
			if h[j], err = rd(); err != nil {
				return nil, err
			}
		}
		if h[4] > 1<<16 {
			return nil, errors.New("histgrm: bucket count out of range")
		}
		c := Category{ID: h[0], Total: h[1], Min: h[2], Max: h[3], Buckets: make([]uint32, h[4])}
		for j := range c.Buckets {
			if c.Buckets[j], err = rd(); err != nil {
				return nil, err
			}
		}
		out = append(out, c)
	}
	return out, nil
}
