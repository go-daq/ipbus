package ipbus

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// Response to a control transaction
type Response struct {
	Err   error
	Code  infoCode
	Data  []uint32
	DataB []byte
}

type transaction struct {
	Type                 typeID
	NWords               uint8
	Addr                 uint32
	Input                []uint32
	resp                 chan Response
	byteslice, closechan bool
}

type packet struct {
	id				uint16
	transactions    []transaction
	reqcap, respcap uint
	reqlen, resplen uint
	request         *bytes.Buffer
	sent			time.Time
}

func (p packet) Bytes() []byte {
	b := p.request.Bytes()
	binary.BigEndian.PutUint16(b[2:4], p.id)
	return b
}

func emptypacket(pt packetType) packet {
	trans := make([]transaction, 0, 8)
	request := bytes.NewBuffer(make([]byte, 0, 1472))
	header := uint32(0)
	header |= protocolversion << 24
	header |= 0xf0
	header |= uint32(pt)
	binary.Write(request, binary.BigEndian, header)
	// Normal IP packet has up to 1500 bytes. IP header is 20 bytes, UDP
	// header is 8 bytes. This leaves 368 words for the ipbus data.
	size := (MaxPacketSize - 28) / 4
	return packet{0, trans, size, size, 0, 0, request, time.Time{}} // For normal packet
}

func (p *packet) add(trans transaction) error {
	// Check that the size of the transaction and its reply will fit and that
	// the request has the correct amount of data.
	// Update the reqlen and resplen
	reqspace, respspace := p.space()
	switch {
	case trans.Type == read || trans.Type == readnoninc:
		if len(trans.Input) > 0 {
			return fmt.Errorf("Read/ReadNonInc transaction with nonzero (%d) words of input data", len(trans.Input))
		}
		if reqspace < 2 || respspace < uint(trans.NWords+1) {
			return fmt.Errorf("Add %d word Read[NonInc]: insufficient space in packet.", trans.NWords)
		}
		p.reqlen += 2
		p.resplen += uint(trans.NWords + 1)
	case trans.Type == write || trans.Type == writenoninc:
		if len(trans.Input) != int(trans.NWords) {
			return fmt.Errorf("Write/WriteNonInc transaction with NWords = %d, but %d words of input data", trans.NWords, len(trans.Input))
		}
		if reqspace < uint(2+trans.NWords) || respspace < 1 {
			return fmt.Errorf("Add %d word Write[NonInc]: insufficient space in packet.", trans.NWords)
		}
		p.reqlen += uint(2 + trans.NWords)
		p.resplen += 1
	case trans.Type == rmwbits:
		if len(trans.Input) != 2 {
			return fmt.Errorf("RMWbits with %d words of input data (expect 2: AND, OR)", len(trans.Input))
		}
		if reqspace < 4 || respspace < 2 {
			return fmt.Errorf("Add RMWbits: insufficient space in packet.")
		}
		p.reqlen += 4
		p.resplen += 2
	case trans.Type == rmwsum:
		if len(trans.Input) != 1 {
			return fmt.Errorf("RMWsum with %d words of input data (expect 1: ANDEND)", len(trans.Input))
		}
		if reqspace < 3 || respspace < 2 {
			return fmt.Errorf("Add RMWsum: insufficient space in packet.")
		}
		p.reqlen += 3
		p.resplen += 2
	}

	// Fill the outgoing packet
	header := uint32(0)
	header |= protocolversion << 24
	tID := len(p.transactions)
	header |= uint32(tID) << 16
	header |= uint32(trans.NWords) << 8
	header |= uint32(trans.Type) << 4
	header |= uint32(request)
	binary.Write(p.request, binary.BigEndian, header)
	binary.Write(p.request, binary.BigEndian, trans.Addr)
	binary.Write(p.request, binary.BigEndian, trans.Input)
	p.transactions = append(p.transactions, trans)
	return error(nil)
}

func (p packet) space() (uint, uint) {
	return p.reqcap - p.reqlen, p.respcap - p.reqlen
}

type usrrequest struct {
	typeid    typeID
	nwords    uint
	addr      uint32
	Input     []uint32
	resp      chan Response
	byteslice bool
	dispatch  bool
}
