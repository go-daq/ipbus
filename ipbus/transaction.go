package ipbus

import (
	//	"bytes"
	"encoding/binary"
	"fmt"
	"time"
)

// Response to a control transaction
type Response struct {
	Err   error
	Code  InfoCode
	Data  []uint32
	DataB []byte
}

type transaction struct {
	outheader transactionheader
	//	Type                 typeID
	//	NWords               uint8
	Addr                 uint32
	Input                []uint32
	resp                 chan Response
	byteslice, closechan bool
}

func newrequesttransaction(tid typeID, words uint8, addr uint32, input []uint32, resp chan Response, byteslice, final bool) transaction {
	header := transactionheader{uint8(protocolversion), 0x0, words, tid, Request}
	trans := transaction{header, addr, input, resp, byteslice, final}
	return trans
}

type packet struct {
	header          packetheader
	id              uint16
	transactions    []transaction
	replies         []Response
	reqcap, respcap uint
	reqlen, resplen uint
	//	request         *bytes.Buffer
	request []byte
	sent    time.Time
}

func (p packet) String() string {
	return fmt.Sprintf("packet id = %d, request = %x", p.id, p.request)
}

func (p packet) Bytes() []byte {
	/*
		b := p.request.Bytes()
		binary.BigEndian.PutUint16(b[2:4], p.id)
		return b
	*/
	return p.request
}

// Parse an IPbus reply byte stream into responses
func (p *packet) parse(data []byte) error {
	packheader, err := newPacketHeader(data)
	if err != nil {
		for _ = range p.transactions {
			resp := Response{err, 0xe, nil, nil}
			p.replies = append(p.replies, resp)

		}
		return err
	}
	if packheader.ptype == control {
		data = data[4:]
		for len(data) > 0 {
			transheader, err := newTransactionHeader(data, packheader.order)
			trans := p.transactions[transheader.id]
			data = data[4:]
			resp := Response{err, transheader.code, nil, nil}
			if err == nil { // heard successfully parsed
				if transheader.code == Success {
					switch {
					case transheader.tid == read || transheader.tid == readnoninc:
						if trans.byteslice {
							resp.DataB = data[:int(transheader.words)*4]
						} else {
							resp.Data = bytes2uint32s(data[:int(transheader.words)*4], packheader.order)
						}
						fmt.Printf("Skipping %d words, %d bytes.\n", transheader.words, 4 * int(transheader.words))
						data = data[int(transheader.words)*4:]
					case transheader.tid == write || transheader.tid == writenoninc:
						if trans.byteslice {
							resp.DataB = []byte{}
						} else {
							resp.Data = []uint32{}
						}
					case transheader.tid == rmwbits || transheader.tid == rmwsum:
						if trans.byteslice {
							resp.DataB = data[:4]
						} else {
							resp.Data = bytes2uint32s(data[:4], packheader.order)
						}
						data = data[4:]
					}
				} else { // info code is not success
					fmt.Printf("Received a transaction info code: %v\n", transheader.code)
					resp.Err = fmt.Errorf("IPbus error: %s", transactionerrs[transheader.code])
					// What do I do if there was an error, stop parsing now?
					// Does it continue with replies to following transactions?
					// Need to check IPbus docs.
				}
			}
			p.replies = append(p.replies, resp)
		}
		if len(p.replies) < len(p.transactions) {
			err := fmt.Errorf("Did not receive sufficient bytes.")
			for i := len(p.replies); i < len(p.transactions); i++ {
				resp := Response{err, 0xe, nil, nil}
				p.replies = append(p.replies, resp)
			}
		}
		return nil
	} else if packheader.ptype == status {
		// Need to do something special to parse status packet

	} else if packheader.ptype == resend {
		return fmt.Errorf("IPbus client shouldn't receive a resend request type packet.")
	} else {
		return fmt.Errorf("Packet has invalid type: 0x%x", packheader.ptype)
	}
	return nil
}

func byteorder(header []byte) (binary.ByteOrder, error) {
	if len(header) < 4 {
		return nil, fmt.Errorf("Cannot identify byte order, header (0x%x) too short", header)
	}
	v := uint8(protocolversion) << 4
	boq := uint8(0xf0) // byte order qualifier
	if (header[0] == v) && ((header[3] & boq) == boq) {
		return binary.BigEndian, nil
	} else if (header[3] == v) && ((header[0] & boq) == boq) {
		return binary.LittleEndian, nil
	} else {
		return nil, fmt.Errorf("Invalid header: %x, cannot find endianess.", header)
	}
}

// Send replies back over correct channels.
func (p packet) send() {
	for i, tr := range p.transactions {
		tr.resp <- p.replies[i]
		if tr.closechan {
			close(tr.resp)
		}
	}
}

func emptypacket(pt packetType) *packet {
	trans := make([]transaction, 0, 8)
	replies := make([]Response, 0, 8)
	//request := bytes.NewBuffer(make([]byte, 0, 1472))
	request := make([]byte, 4, 1472)
	/*
		header := uint32(0)
		header |= protocolversion << 24
		header |= 0xf0
		header |= uint32(pt)
		binary.Write(request, binary.BigEndian, header)
	*/
	// Normal IP packet has up to 1500 bytes. IP header is 20 bytes, UDP
	// header is 8 bytes. This leaves 368 words for the ipbus data.
	size := (MaxPacketSize - 28) / 4
	header := packetheader{uint8(protocolversion), uint16(0),
			  pt, defaultorder}
	return &packet{header, 0, trans, replies, size, size, 0, 0, request, time.Time{}} // For normal packet
}

func (p *packet) add(trans transaction) error {
	// Check that the size of the transaction and its reply will fit and that
	// the request has the correct amount of data.
	// Update the reqlen and resplen
	reqspace, respspace := p.space()
	switch {
	case trans.outheader.tid == read || trans.outheader.tid == readnoninc:
		if len(trans.Input) > 0 {
			return fmt.Errorf("Read/ReadNonInc transaction with nonzero (%d) words of input data", len(trans.Input))
		}
		if reqspace < 2 || respspace < uint(trans.outheader.words+1) {
			return fmt.Errorf("Add %d word Read[NonInc]: insufficient space in packet.", trans.outheader.words)
		}
		p.reqlen += 2
		p.resplen += uint(trans.outheader.words + 1)
	case trans.outheader.tid == write || trans.outheader.tid == writenoninc:
		if len(trans.Input) != int(trans.outheader.words) {
			return fmt.Errorf("Write/WriteNonInc transaction with NWords = %d, but %d words of input data", trans.outheader.words, len(trans.Input))
		}
		if reqspace < uint(2+trans.outheader.words) || respspace < 1 {
			return fmt.Errorf("Add %d word Write[NonInc]: insufficient space in packet.", trans.outheader.words)
		}
		p.reqlen += uint(2 + trans.outheader.words)
		p.resplen += 1
	case trans.outheader.tid == rmwbits:
		if len(trans.Input) != 2 {
			return fmt.Errorf("RMWbits with %d words of input data (expect 2: AND, OR)", len(trans.Input))
		}
		if reqspace < 4 || respspace < 2 {
			return fmt.Errorf("Add RMWbits: insufficient space in packet.")
		}
		p.reqlen += 4
		p.resplen += 2
	case trans.outheader.tid == rmwsum:
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
	transhead := []byte{0, 0, 0, 0}
	fmt.Printf("0: header order = %v\n", p.header.order)
	err := trans.outheader.encode(transhead, p.header.order)
	if err != nil {
		fmt.Printf("Error encoding transaction header: %v\n", err)
	}
	p.request = append(p.request, transhead...)
	fmt.Printf("Added transaction header (%x): p.request = %x\n", transhead, p.request)
	data := make([]byte, 4*len(trans.Input) + 4)
	fmt.Printf("data [%d, %d] = %v\n", len(data), cap(data), data)
	p.header.order.PutUint32(data, trans.Addr)
	for i, val := range trans.Input {
		fmt.Printf("i = %d, val = %d = 0x%x\n", i, val, val)
		buffer := data[(i + 1) * 4:]
		fmt.Printf("Putting 0x%x into %v\n", val, buffer)
		fmt.Printf("1: header order = %v\n", p.header.order)
		p.header.order.PutUint32(buffer, val)
	}
	fmt.Printf("data = %x\n", data)
	p.request = append(p.request, data...)
	fmt.Printf("Added data: p.request = %x\n", p.request)
	p.transactions = append(p.transactions, trans)
	return error(nil)
}

func (p packet) space() (uint, uint) {
	return p.reqcap - p.reqlen, p.respcap - p.reqlen
}

func (p *packet) writeheader(id uint16) error {
	p.header.pid = id
	p.id = id
	fmt.Printf("Before writing header p.request = %x\n", p.request)
	err := p.header.encode(p.request)
	fmt.Printf("Wrote header with id = 0x%x: %x\n", id, p.request)
	return err
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
