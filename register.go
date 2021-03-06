// Copyright 2018 The go-daq Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipbus

import (
	"fmt"
)

func newmask(name string, value uint32) msk {
	shift := uint(0)
	for i := uint(0); i < 32; i++ {
		test := uint32(1) << i
		if test&value > 0 {
			shift = i
			break
		}
	}
	return msk{name, value, shift}
}

type msk struct {
	name  string
	value uint32
	shift uint
}

type Register struct {
	Name   string
	Addr   uint32   // Global IPbus address
	Masks  []string // List of names bitmasks
	noninc bool
	size   int
	msks   map[string]msk
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
	for n, m := range r.msks {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("%s:0x%x", n, m.value)
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
