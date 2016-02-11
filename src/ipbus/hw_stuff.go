package ipbus

import (
	"data"
	"fmt"
	"net"
	oldipbus "old/ipbus"
)

type hwpacket struct {
	Data  []byte
	RAddr net.Addr
}

func emptyPacket() hwpacket {
	d := make([]byte, 1500)
	return hwpacket{Data: d}
}

func newPacket(data []byte) hwpacket {
	return hwpacket{Data: data}
}

type hwrequest struct {
	request oldipbus.Packet
	reqresp data.ReqResp
	dest    chan data.ReqResp
}

func (r hwrequest) String() string {
	return fmt.Sprintf("reqresp: index = %d, size = %d [%x]", r.reqresp.RespIndex, r.reqresp.RespSize, r.reqresp.Bytes)
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
