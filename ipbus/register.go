package ipbus

import (
	"fmt"
)

type Register struct {
	Name   string
	Addr   uint32
	Masks  map[string]uint32
	noninc bool
	size   int
}

func (r Register) MaskedValue(mask string, value uint32) (uint32, error) {
	m, ok := r.Masks[mask]
	if !ok {
		return uint32(0), fmt.Errorf("MaskedValue: mask '%s' not found", mask)
	}
	shift := uint(0)
	for (0x1 << shift) & m == 0 {
		shift++
	}
	value &= m
	value = value >> shift
	return value, nil
}

func (r Register) String() string {
	s := fmt.Sprintf("%s at 0x%x", r.Name, r.Addr)
	if r.noninc {
		s += " (non-inc)"
	}
	if len(r.Masks) > 0 {
		s += " ["
	}
	i := 0
	for n, m := range r.Masks {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s:0x%x", n, m)
		i += 1
	}
	if i > 0 {
		s += "]"
	}
	return s
}
