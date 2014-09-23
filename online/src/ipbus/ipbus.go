package ipbus

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const Version uint8 = 2

type InfoCode uint8

var Success InfoCode = 0x0
var BadHeader InfoCode = 0x1
var BusReadError InfoCode = 0x4
var BusWriteError InfoCode = 0x5
var BusReadTimeout InfoCode = 0x6
var BusWriteTimeout InfoCode = 0x7
var Request InfoCode = 0xf

var goodcodes map[InfoCode]bool = map[InfoCode]bool{
	Success:         true,
	BadHeader:       true,
	BusReadError:    true,
	BusWriteError:   true,
	BusReadTimeout:  true,
	BusWriteTimeout: true,
	Request:         true,
}

type TypeID uint8

var Read TypeID = 0x0
var Write TypeID = 0x1
var ReadNonInc TypeID = 0x2
var WriteNonInc TypeID = 0x3
var RMWbits TypeID = 0x4
var RMWsum TypeID = 0x5

var goodtypeids map[TypeID]bool = map[TypeID]bool{
	Read:        true,
	Write:       true,
	ReadNonInc:  true,
	WriteNonInc: true,
	RMWbits:     true,
	RMWsum:      true,
}

type Transaction struct {
	header  uint32
	Version uint8
	ID      uint16
	Words   uint8
	Type    TypeID
	Code    InfoCode
	Body    []byte
}

func (t Transaction) String() string {
	return fmt.Sprintf("v%d, id = %d, %d words, type = %d, code = %d. <%x>", t.Version, t.ID, t.Words, t.Type, t.Code, t.Body)
}

func (t Transaction) Encode() ([]byte, error) {
	data := make([]byte, 0, len(t.Body)+4)
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, t.header); err != nil {
		return data, err
	}
	data = append(data, buf.Bytes()...)
	data = append(data, t.Body...)
	return data, error(nil)
}

func (t *Transaction) DecodeHeader(header []byte) error {
	if len(header) != 4 {
		return fmt.Errorf("Received %d byte header.", len(header))
	}
	h := uint32(0)
	for i, b := range header {
		h |= (uint32(b) << (24 - (8 * uint(i))))
	}
	//fmt.Printf("Got header = %x\n", h)
	t.header = h
	v := uint8((h & 0xf0000000) >> 28)
	id := uint16((h & 0x0fff0000) >> 16)
	words := uint8((h & 0x0000ff00) >> 8)
	typeid := TypeID((h & 0x000000f0) >> 4)
	if _, ok := goodtypeids[typeid]; !ok {
		return errors.New("Invalid TypeID.")
	}
	code := InfoCode(h & 0x0000000f)
	if _, ok := goodcodes[code]; !ok {
		return fmt.Errorf("Invalid InfoCode: %d.", code)
	}
	t.Version = v
	t.ID = id
	t.Words = words
	t.Type = typeid
	t.Code = code
	return error(nil)
}

func (t *Transaction) Decode(header []byte, body []byte) error {
	if err := t.DecodeHeader(header); err != nil {
		return err
	}
	if len(body)%4 != 0 {
		return fmt.Errorf("Ivalid body size: %d", len(body))
	}
	t.Body = body
	return error(nil)
}

func MakeTransaction(version uint8, id uint16, words uint8, tid TypeID, code InfoCode, body []byte) Transaction {
	header := uint32(code)
	header |= (uint32(tid) << 4)
	header |= (uint32(words) << 8)
	header |= (uint32(id) << 16)
	header |= (uint32(version) << 28)
	t := Transaction{header, version, id, words, tid, code, body}
	return t
}

func MakeRead(size uint8, addr uint32) Transaction {
	body := make([]byte, 4)
	body[0] = byte(addr & 0xff)
	body[1] = byte((addr & 0xff00) >> 8)
	body[2] = byte((addr & 0xff0000) >> 16)
	body[3] = byte((addr & 0xff000000) >> 24)
	return MakeTransaction(Version, 0, size, Read, Request, body)
}

func MakeReadReply(data []byte) Transaction {
	//fmt.Printf("Making reply with %d bytes -> %d words.\n", len(data), len(data) / 4)
	return MakeTransaction(Version, 0, uint8(len(data)/4), Read, Success, data)
}

func MakeReadNonInc(size uint8, addr uint32) Transaction {
	body := make([]byte, 4)
	body[0] = byte(addr & 0xf)
	body[1] = byte((addr & 0xf0) >> 8)
	body[2] = byte((addr & 0xf00) >> 16)
	body[3] = byte((addr & 0xf000) >> 24)
	return MakeTransaction(Version, 0, size, ReadNonInc, Request, body)
}

func MakeWrite(addr uint32, data []byte) Transaction {
	body := make([]byte, 4, 4+len(data))
	body[0] = byte(addr & 0xf)
	body[1] = byte((addr & 0xf0) >> 8)
	body[2] = byte((addr & 0xf00) >> 16)
	body[3] = byte((addr & 0xf000) >> 24)
	body = append(body, data...)
	return MakeTransaction(Version, 0, uint8(len(data)), Write, Request, body)
}

func MakeWriteNonInc(addr uint32, data []byte) Transaction {
	body := make([]byte, 4, 4+len(data))
	body[0] = byte(addr & 0xff)
	body[1] = byte((addr & 0xff00) >> 8)
	body[2] = byte((addr & 0xff0000) >> 16)
	body[3] = byte((addr & 0xff000000) >> 24)
	body = append(body, data...)
	return MakeTransaction(Version, 0, uint8(len(data)), WriteNonInc, Request, body)
}

func MakeRMWbits(addr uint32, and uint32, or uint32) Transaction {
	body := make([]byte, 12)
	for i := uint(0); i < 4; i++ {
		body[i] = byte((addr & (0xff << (8 * i))) >> (8 * i))
		body[i+4] = byte((and & (0xff << (8 * i))) >> (8 * i))
		body[i+8] = byte((or & (0xff << (8 * i))) >> (8 * i))
	}
	return MakeTransaction(Version, 0, 1, RMWbits, Request, body)
}

func MakeRMWsum(addr uint32, addend uint32) Transaction {
	body := make([]byte, 8)
	for i := uint(0); i < 4; i++ {
		body[i] = byte((addr & (0xff << (8 * i))) >> (8 * i))
		body[i+4] = byte((addend & (0xff << (8 * i))) >> (8 * i))
	}
	return MakeTransaction(Version, 0, 1, RMWsum, Request, body)
}

type PacketType uint8

const (
	Control PacketType = 0x0
	Status  PacketType = 0x1
	Resend  PacketType = 0x2
)

var goodpackettypes map[PacketType]bool = map[PacketType]bool{
	Control: true,
	Status:  true,
	Resend:  true,
}

type Packet struct {
	header       uint32
	Version      uint8
	ID           uint16
	Type         PacketType
	Transactions []Transaction
}

func (p Packet) Encode() ([]byte, error) {
	data := []byte{}
	buf := new(bytes.Buffer)
	//fmt.Printf("p.header = %x\n", p.header)
	if err := binary.Write(buf, binary.BigEndian, p.header); err != nil {
		return data, err
	}
	data = append(data, buf.Bytes()...)
	for _, t := range p.Transactions {
		d, err := t.Encode()
		if err != nil {
			return data, err
		}
		data = append(data, d...)
	}
	return data, error(nil)
}

func MakePacket(v uint8, id uint16, pt PacketType) Packet {
	header := uint32(pt)
	header |= (uint32(0xf) << 4)
	header |= (uint32(id) << 8)
	header |= (uint32(v) << 28)
	p := Packet{header, v, id, pt, []Transaction{}}
	return p
}

func (p *Packet) DecodeHeader(h []byte) error {
	if len(h) != 4 {
		return fmt.Errorf("Received %d byte header.", len(h))
	}
	header := uint32(0)
	for i, b := range h {
		header |= (uint32(b) << (24 - (8 * uint(i))))
	}
	//fmt.Printf("Got header: %x\n", header)
	p.header = header
	p.Version = uint8((header & 0xf0000000) >> 28)
	p.ID = uint16((header & 0x00ffff00) >> 8)
	bo := uint8((header & 0x000000f0) >> 4)
	if bo != 0xf {
		return fmt.Errorf("Incorrect byte order: 0x%x", bo)
	}
	p.Type = PacketType((header & 0x0000000f))
	if _, ok := goodpackettypes[p.Type]; !ok {
		return fmt.Errorf("Invalid packet type: %v", p.Type)
	}
	return error(nil)
}

func (p *Packet) Decode(data []byte) error {
	if len(data) < 4 {
		return errors.New("Empty data, cannot decode packet.")
	}
	if err := p.DecodeHeader(data[:4]); err != nil {
		panic(err)
	}
	//fmt.Printf("received header: %v\nReading words from %d bytes.\n", p, len(data) - 4)
	nword := 1
	for nword < len(data)/4 {
		//fmt.Printf("nword = %d, len(data) = %d\n", nword, len(data))
		// Store the transaction header in val
		//fmt.Printf("transaction header = %x\n", val)
		t := Transaction{}
		if err := t.DecodeHeader(data[4*nword : 4*(nword+1)]); err != nil {
			return err
		}
		nword += 1
		nbody := 0
		if t.Code == Request {
			if t.Type == Read || t.Type == ReadNonInc {
				nbody = 1
			} else if t.Type == Write || t.Type == WriteNonInc {
				nbody = int(t.Words + 1)
			} else if t.Type == RMWbits {
				nbody = 3
			} else if t.Type == RMWsum {
				nbody = 2
			}
		} else if t.Code == Success {
			if t.Type == Read || t.Type == ReadNonInc {
				//fmt.Printf("Received read success: %d words.\n", int(t.Words))
				nbody = int(t.Words)
			} else if t.Type == Write || t.Type == WriteNonInc {
				nbody = 0
			} else if t.Type == RMWbits {
				nbody = 1
			} else if t.Type == RMWsum {
				nbody = 1
			}
		} else {
			panic(fmt.Errorf("Non implemented code: %x", t.Code))
		}
		//fmt.Printf("Going to read %d words for body.\n", nbody)
		t.Body = data[nword*4 : nword*4+nbody*4]
		nword += nbody
		p.Transactions = append(p.Transactions, t)
		//fmt.Printf("Added transaction: %v\n", p)
		//fmt.Printf("end of loop: nword = %d, len(data) = %d\n", nword, len(data))
	}
	return error(nil)
}

// Use these just to work out what is in the data, if it's for storing, etc
type PackHeader struct {
	Version uint8
	ID      uint16
	Type    PacketType
	Trans   []TranHeader
}

func (p *PackHeader) Parse(data *[]byte) error {
	p.Version = uint8(((*data)[0] | 0xf0) >> 4)
	p.ID = (uint16((*data)[1]) << 8)
	p.ID |= uint16((*data)[2])
	if (*data)[3]&0xf0 != 0xf0 {
		return fmt.Errorf("Invalid byte order in: %x", (*data)[3])
	}
	p.Type = PacketType((*data)[3] & 0x0f)
	if _, ok := goodpackettypes[p.Type]; !ok {
		return fmt.Errorf("Invalid packet type: %v", p.Type)
	}
	loc := 4
	nbytes := len(*data)
	for loc < nbytes {
		th := TranHeader{}
		err := th.Parse(data, loc)
		if err != nil {
			return err
		}
		p.Trans = append(p.Trans, th)
		if th.Code == Success {
			if th.Type == Read || th.Type == ReadNonInc {
				nbytes += 8
			} else if th.Type == Write || th.Type == WriteNonInc {
				nbytes += 4 * (int(th.Words) + 2)
			} else if th.Type == RMWbits {
				nbytes += 16
			} else if th.Type == RMWsum {
				nbytes += 12
			}
		} else if th.Code == Request {
			if th.Type == Read || th.Type == ReadNonInc {
				nbytes += 4 * (int(th.Words) + 1)
			} else if th.Type == Write || th.Type == WriteNonInc {
				nbytes += 4
			} else if th.Type == RMWbits || th.Type == RMWsum {
				nbytes += 8
			}
		} else {
			return fmt.Errorf("Transaction code %x not implimented.", th.Code)
		}
	}
	return error(nil)
}

type TranHeader struct {
	Loc     int
	Version uint8
	ID      uint16
	Words   uint8
	Type    TypeID
	Code    InfoCode
}

func (t *TranHeader) Parse(data *[]byte, loc int) error {
	t.Loc = loc
	t.Version = uint8(((*data)[0] | 0xf0) >> 4)
	t.ID = (uint16((*data)[0]&0x0f) << 16)
	t.Words |= uint8((*data)[1])
	t.Type = TypeID(((*data)[3] & 0xf0) >> 4)
	if _, ok := goodtypeids[t.Type]; !ok {
		return fmt.Errorf("Invalid transaction type: %v", t.Type)
	}
	t.Code = InfoCode(((*data)[3] & 0x0f))
	if _, ok := goodcodes[t.Code]; !ok {
		return fmt.Errorf("Invalid Transaction code: %v", t.Code)
	}
	return error(nil)
}
