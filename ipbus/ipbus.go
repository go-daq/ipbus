// Package ipbus enables communication with FPGAs using the IPbus UDP protocol.
package ipbus

import (
	"encoding/binary"
	"fmt"
)

// Supported IPbus protocol version
const IPbusVersion = 2.0
const protocolversion = uint32(2)

// Maxiumum Ethernet packet size (bytes)
var MaxPacketSize = uint(1500)

// Information codes
type InfoCode uint8

const Success InfoCode = 0x0
const BadHeader InfoCode = 0x1
const BusReadError InfoCode = 0x4
const BusWriteError InfoCode = 0x5
const BusReadTimeout InfoCode = 0x6
const busWriteTimeout InfoCode = 0x7
const Request InfoCode = 0xf

var transactionerrs = []string{"Success", "Bad Header", "Bus Read Error", "Bus Write Error", "Bus Read Timeout", "bus Write Timeout"}

// Transaction types
type typeID uint8

const read typeID = 0x0
const write typeID = 0x1
const readnoninc typeID = 0x2
const writenoninc typeID = 0x3
const rmwbits typeID = 0x4
const rmwsum typeID = 0x5

func byte2uint32(bs []byte, order binary.ByteOrder) uint32 {
	return order.Uint32(bs)
}

func bytes2uint32s(bs []byte, order binary.ByteOrder) []uint32 {
	size := len(bs) / 4
	us := make([]uint32, 0, size)
	for i := 0; i < size; i++ {
		us = append(us, order.Uint32(bs))
		bs = bs[4:]
	}
	return us
}

// Packet types
type packetType uint8

const control packetType = 0x0
const status packetType = 0x1
const resend packetType = 0x2

type packetheader struct {
	version uint8
	pid     uint16
	ptype   packetType
	order   binary.ByteOrder
}

func newPacketHeader(data []byte) (packetheader, error) {
	p := packetheader{}
	err := p.decode(data)
	return p, err
}

func (p *packetheader) decode(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("Packet header must be four bytes.")
	}
	v := uint8(protocolversion) << 4
	boq := uint8(0xf0) // byte order qualifier
	if (data[0] == v) && ((data[3] & boq) == boq) {
		p.version = uint8(protocolversion)
		p.pid = uint16(data[1]) << 8
		p.pid |= uint16(data[2])
		p.ptype = packetType(data[3] & 0x0f)
		p.order = binary.BigEndian
		return nil
	} else if (data[3] == v) && ((data[0] & boq) == boq) {
		p.version = uint8(protocolversion)
		p.pid = uint16(data[2]) << 8
		p.pid |= uint16(data[1])
		p.ptype = packetType(data[0] & 0x0f)
		p.order = binary.LittleEndian
		return nil
	} else {
		return fmt.Errorf("Invalid packet header: 0x%x", data[0:4])
	}
}

func (p packetheader) encode(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("Four bytes required to store packet header.")
	}
	if p.order == binary.BigEndian {
		data[0] = p.version << 4
		data[1] = uint8((p.pid & 0xff00) >> 8)
		data[2] = uint8(p.pid & 0x00ff)
		data[3] = 0xf0 & uint8(p.ptype)
	} else if p.order == binary.LittleEndian {
		data[3] = p.version << 4
		data[2] = uint8((p.pid & 0xff00) >> 8)
		data[1] = uint8(p.pid & 0x00ff)
		data[0] = 0xf0 & uint8(p.ptype)
	} else {
		return fmt.Errorf("Cannot write invalid packet header to byte slice.")
	}
	return nil
}

type transactionheader struct {
	version uint8
	id      uint16
	words   uint8
	tid     typeID
	code    InfoCode
}

func newTransactionHeader(data []byte, order binary.ByteOrder) (transactionheader, error) {
	t := transactionheader{}
	err := t.decode(data, order)
	return t, err
}

func (th *transactionheader) decode(data []byte, order binary.ByteOrder) error {
	if len(data) < 4 {
		return fmt.Errorf("Transaction header must be four bytes.")
	}
	if order == binary.BigEndian {
		th.version = uint8(data[0] >> 4)
		th.id = uint16(data[1])
		th.id |= uint16(data[0]&0x0f) << 8
		th.words = uint8(data[2])
		th.tid = typeID((data[3] & 0xf0) >> 4)
		th.code = InfoCode(data[3] & 0x0f)
		return nil
	} else if order == binary.LittleEndian {
		th.version = uint8(data[3] >> 4)
		th.id = uint16(data[2])
		th.id |= uint16(data[3]&0x0f) << 8
		th.words = uint8(data[1])
		th.tid = typeID((data[0] & 0xf0) >> 4)
		th.code = InfoCode(data[0] & 0x0f)
		return nil
	} else {
		return fmt.Errorf("Invalid byte order to decode transaction header.")
	}
}

func (th transactionheader) encode(data []byte, order binary.ByteOrder) error {
	if len(data) < 4 {
		return fmt.Errorf("Four bytes required to store transaction header.")
	}
	if order == binary.BigEndian {
		data[0] = (uint8(th.version) << 4) | uint8((th.id&0x0f00)>>8)
		data[1] = uint8(th.id & 0xff)
		data[2] = th.words
		data[1] = uint8(th.tid)<<4 | uint8(th.code)
	} else if order == binary.LittleEndian {
		data[3] = (uint8(th.version) << 4) | uint8((th.id&0x0f00)>>8)
		data[2] = uint8(th.id & 0xff)
		data[1] = th.words
		data[0] = uint8(th.tid)<<4 | uint8(th.code)
	} else {
		return fmt.Errorf("Invalid byte order to write transaction header to byte slice.")
	}
	return nil
}
