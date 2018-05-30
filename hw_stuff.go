// Copyright 2018 The go-daq Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ipbus

import (
	"encoding/binary"
	"fmt"
	"net"
)

func newResendPacket(id uint16) []byte {
	data := make([]byte, 4)
	data[0] = uint8(protocolversion) << 4
	data[1] = uint8(id >> 8)
	data[2] = uint8(id & 0x00ff)
	boq := uint8(0xf0)
	data[3] = boq | uint8(resend)
	return data
}

func newStatusPacket() []byte {
	data := make([]byte, 64)
	data[0] = uint8(protocolversion) << 4
	boq := uint8(0xf0)
	data[3] = boq | uint8(status)
	return data
}

type hwpacket struct {
	Data   []byte
	RAddr  net.Addr
	header packetheader
}

type targetstatus struct {
	mtu             uint32
	nresponsebuffer uint32
	nextid          uint16
	received, sent  []packetheader
}

func parseStatus(data []byte) (targetstatus, error) {
	mtu := byte2uint32(data[4:8], binary.BigEndian)
	nresponsebuffer := byte2uint32(data[8:12], binary.BigEndian)
	nextheader := byte2uint32(data[12:16], binary.BigEndian)
	nextid := uint16((nextheader & 0x00ffff00) >> 8)
	received := make([]packetheader, 0, 4)
	for i := 0; i < 4; i++ {
		index := 32 + 4*i
		header, err := newPacketHeader(data[index : index+4])
		if err == nil {
			received = append(received, header)
		}
	}
	sent := make([]packetheader, 0, 4)
	for i := 0; i < 4; i++ {
		index := 48 + 4*i
		header, err := newPacketHeader(data[index : index+4])
		if err == nil {
			sent = append(sent, header)
		}
	}
	ts := targetstatus{mtu, nresponsebuffer, nextid, received, sent}
	return ts, error(nil)
}

func emptyPacket() hwpacket {
	d := make([]byte, 1500)
	return hwpacket{Data: d}
}

func newPacket(data []byte) hwpacket {
	return hwpacket{Data: data}
}

func newTracker(size int) tracker {
	ids := make([]uint16, size)
	return tracker{ids, 0, size}
}

type tracker struct {
	ids        []uint16
	index, max int
}

func (t tracker) String() string {
	return fmt.Sprintf("%v, %d", t.ids, t.index)
}

func (t *tracker) add(id uint16) {
	t.index = (t.index + 1) % t.max
	t.ids[t.index] = id
}

type idlog struct {
	ids           []uint16
	first, n, max int
}

func (i *idlog) add(id uint16) error {
	if id == 0 {
		return fmt.Errorf("Cannot add id = 0 to id logger.")
	}
	if i.n == i.max {
		return fmt.Errorf("Cannot add id = %d to full id logger.", id)
	}
	next := (i.first + i.n) % i.max
	i.ids[next] = id
	i.n += 1
	return error(nil)
}

func (i *idlog) remove() error {
	if i.n == 0 {
		return fmt.Errorf("Cannot remove id from empty id logger.")
	}
	i.first = (i.first + 1) % i.max
	i.n -= 1
	return error(nil)
}

func (i *idlog) oldest() (uint16, bool) {
	return i.ids[i.first], i.n > 0
}

func (i *idlog) secondoldest() (uint16, bool) {
	next := (i.first + 1) % i.max
	return i.ids[next], i.n > 1
}

func (i *idlog) newest() (uint16, bool) {
	newest := (i.first + i.n) % i.max
	return i.ids[newest], i.n > 0
}

func (i *idlog) sorted() []uint16 {
	vals := make([]uint16, 0, i.n)
	for j := 0; j < i.n; j++ {
		next := (i.first + j) % i.max
		vals = append(vals, i.ids[next])
	}
	return vals
}

func (i idlog) String() string {
	return fmt.Sprintf("ids: %v, first = %d, n = %d", i.ids, i.first, i.n)
}

func newIDLog(size int) idlog {
	ids := make([]uint16, size)
	return idlog{ids: ids, first: 0, n: 0, max: size}
}

type packidok struct {
	pack  *packet
	id    uint16
	ok    bool
	reply chan packidok
}

func newpacketlog() *packetlog {
	pks := make(map[uint16]*packet)
	chadd := make(chan packidok)
	chget := make(chan packidok)
	chgetall := make(chan packidok)
	chremove := make(chan packidok)
	pl := packetlog{packets: pks, chadd: chadd, chget: chget, chgetall: chgetall, chremove: chremove}
	go pl.run()
	return &pl
}

type packetlog struct {
	packets                          map[uint16]*packet
	chadd, chget, chgetall, chremove chan packidok
	Verbose                          bool
}

func (p *packetlog) run() {
	for {
		select {
		case pk := <-p.chadd:
			// Add a packet to map
			p.packets[pk.id] = pk.pack
		case in := <-p.chget:
			// Get a packet if it exists
			pk, ok := p.packets[in.id]
			in.pack = pk
			in.ok = ok
			in.reply <- in
		case ch := <-p.chgetall:
			// Return range of all current packets
			for id, pack := range p.packets {
				pk := packidok{pack: pack, id: id, ok: true}
				ch.reply <- pk
			}
			close(ch.reply)
		case pk := <-p.chremove:
			// Return range of all current packets
			delete(p.packets, pk.id)
		}
	}
}

func (p *packetlog) add(id uint16, pack *packet) {
	pk := packidok{pack: pack, id: id}
	p.chadd <- pk
}

func (p *packetlog) get(id uint16) (*packet, bool) {
	if p.Verbose {
		fmt.Printf("packetlog.get(%04x)\n", id)
	}
	reply := make(chan packidok)
	pk := packidok{id: id, reply: reply}
	if p.Verbose {
		fmt.Printf("p.chget <- pk\n")
	}
	p.chget <- pk
	if p.Verbose {
		fmt.Printf("rep := <- reply\n")
	}
	rep := <-reply
	if p.Verbose {
		fmt.Printf("packetlog.get(%04x) returning, pack %t\n", id, rep.ok)
	}
	return rep.pack, rep.ok
}

func (p *packetlog) getall() map[uint16]*packet {
	m := make(map[uint16]*packet)
	replies := make(chan packidok, 128)
	pk := packidok{reply: replies}
	p.chgetall <- pk
	for pk := range replies {
		m[pk.id] = pk.pack
	}
	return m
}

func (p *packetlog) remove(id uint16) {
	pk := packidok{id: id}
	p.chremove <- pk
}
