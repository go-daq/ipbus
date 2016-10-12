package ipbus

import (
	"fmt"
)

func newmask(name string, value uint32) msk {
	shift := uint(0)
	for i := uint(0); i < 32; i++ {
		test := uint32(1) << i
		if test & value > 0 {
			shift = i
			break
		}
	}
	return msk{name, value, shift}
}

type msk struct {
	name string
	value uint32
	shift uint
}

type Register struct {
	Name   string
	Addr   uint32 // Global IPbus address
	Masks []string // List of names bitmasks
	noninc bool
	size   int
	msks  map[string]msk
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

// Read the value of masked part of register
func (r Register) ReadMask(mask string, val uint32) (uint32, error) {
	m, ok := r.msks[mask]
	if !ok {
		err := fmt.Errorf("Register %s has no mask %s.", r.Name, mask)
		return uint32(0), err
	}
	maskedval := val & m.value
	maskedval = maskedval >> m.shift
	return maskedval, nil
}

func (r Register) WriteMask(mask string, val uint32) (uint32, uint32, error) {
	m, ok := r.msks[mask]
	if !ok {
		err := fmt.Errorf("Register %s has no mask %s.", r.Name, mask)
		return uint32(0), uint32(0), err
	}
	andterm := uint32(0xffffffff) &^ m.value
	orterm := (val << m.shift) & m.value
	return andterm, orterm, nil
}
