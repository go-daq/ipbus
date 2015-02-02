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
	header := uint32(t.Code)
	header |= (uint32(t.Type) << 4)
	header |= (uint32(t.Words) << 8)
	header |= (uint32(t.ID) << 16)
	header |= (uint32(t.Version) << 28)
	data := make([]byte, 0, len(t.Body)+4)
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.BigEndian, header); err != nil {
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
	body[3] = byte(addr & 0xff)
	body[2] = byte((addr & 0xff00) >> 8)
	body[1] = byte((addr & 0xff0000) >> 16)
	body[0] = byte((addr & 0xff000000) >> 24)
	return MakeTransaction(Version, 0, size, Read, Request, body)
}

func MakeReadReply(data []byte) Transaction {
	//fmt.Printf("Making reply with %d bytes -> %d words.\n", len(data), len(data) / 4)
	return MakeTransaction(Version, 0, uint8(len(data)/4), Read, Success, data)
}

func MakeReadNonInc(size uint8, addr uint32) Transaction {
	body := make([]byte, 4)
	body[3] = byte(addr & 0xff)
	body[2] = byte((addr & 0xff00) >> 8)
	body[1] = byte((addr & 0xff0000) >> 16)
	body[0] = byte((addr & 0xff000000) >> 24)
	return MakeTransaction(Version, 0, size, ReadNonInc, Request, body)
}

func MakeWrite(addr uint32, data []byte) Transaction {
	body := make([]byte, 4, 4+len(data))
	body[3] = byte(addr & 0xff)
	body[2] = byte((addr & 0xff00) >> 8)
	body[1] = byte((addr & 0xff0000) >> 16)
	body[0] = byte((addr & 0xff000000) >> 24)
	body = append(body, data...)
	return MakeTransaction(Version, 0, uint8(len(data) / 4), Write, Request, body)
}

func MakeWriteReply(words uint8) Transaction {
	//fmt.Printf("Making reply with %d bytes -> %d words.\n", len(data), len(data) / 4)
    data := []byte{}
	return MakeTransaction(Version, 0, words, Write, Success, data)
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
		body[i] = byte((addr & (0xff << (8 * (3 - i)))) >> (8 * (3 - i)))
		body[i+4] = byte((and & (0xff << (8 * (3 - i)))) >> (8 * (3 - i)))
		body[i+8] = byte((or & (0xff << (8 * (3 - i)))) >> (8 * (3 - i)))
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
	Version      uint8
	ID           uint16
	Type         PacketType
	Transactions []Transaction
}

func (p Packet) String() string {
    return fmt.Sprintf("v%d, id = %d, t = %v, %v", p.Version, p.ID, p.Type, p.Transactions)
}

func (p *Packet) Add(t Transaction) {
    t.ID = uint16(len(p.Transactions))
    p.Transactions = append(p.Transactions, t)
}

func (p Packet) Encode() ([]byte, error) {
	data := []byte{}
    data = append(data, (uint8(p.Version) << 4))
    data = append(data, (uint8((p.ID & 0xff00) >> 8)))
    data = append(data, (uint8(p.ID & 0xff)))
    data = append(data, (uint8(0xf0 | uint8(p.Type))))
    if p.Type == Status {
        data = append(data, make([]byte, 60)...)
        return data, error(nil)
    }
	for _, t := range p.Transactions {
		d, err := t.Encode()
		if err != nil {
			return data, err
		}
		data = append(data, d...)
	}
	return data, error(nil)
}

func MakePacket(pt PacketType) Packet {
    id := uint16(1)
    if pt != Control {
        id = uint16(0)
    }
	p := Packet{Version, id, pt, []Transaction{}}
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

func (p PackHeader) String() string {
    return fmt.Sprintf("v = %d, id = 0x%x, type = %d, %d transactions: %v.", p.Version, p.ID, p.Type, len(p.Trans), p.Trans)
}

func (p PackHeader) Encode() []byte {
    out := make([]byte, 0, 4)
    out = append(out, uint8(p.Version << 4))
    out = append(out, uint8((p.ID & 0xff00) >> 8))
    out = append(out, uint8(p.ID & 0xff))
    out = append(out, uint8(0xf0) | uint8(p.Type))
    return out
}

func (p *PackHeader) Parse(data []byte, loc int, parsetransactions bool) error {
    //fmt.Printf("Decoding packet of %d bytes, with loc = %d.\n", len(data), loc)
    reverse := false
	p.Version = uint8((data[loc] & 0xf0) >> 4)
	p.ID = (uint16(data[loc + 1]) << 8)
	p.ID |= uint16(data[loc + 2])
	if data[loc + 3] & 0xf0 != 0xf0 {
        if data[loc] & 0xf0 == 0xf0 {
            reverse = true
            fmt.Printf("Header has reverse byte ordering.\n")
        } else {
            return fmt.Errorf("Invalid byte order in: %x = 0x%08x", data[loc:], data[loc + 3])
        }
	}
	p.Type = PacketType(data[loc + 3] & 0x0f)
    if reverse {
        p.Version = uint8((data[loc + 3] & 0xf0) >> 4)
        p.ID = (uint16(data[loc + 2]) << 8)
        p.ID |= uint16(data[loc + 1])
        p.Type = PacketType(data[loc] & 0x0f)
        return error(nil)
    }
	if _, ok := goodpackettypes[p.Type]; !ok {
		return fmt.Errorf("Invalid packet type: %v", p.Type)
	}
    if p.Type == Status {
        fmt.Println("Received a status packet.")
    } else if parsetransactions {
        loc += 4
        nbytes := len(data)
        for loc < nbytes {
            th := TranHeader{}
            err := th.Parse(data, loc)
            if err != nil {
                return err
            }
            p.Trans = append(p.Trans, th)
            if th.Code == Request {
                if th.Type == Read || th.Type == ReadNonInc {
                    loc += 8
                } else if th.Type == Write || th.Type == WriteNonInc {
                    loc += 4 * (int(th.Words) + 2)
                } else if th.Type == RMWbits {
                    loc += 16
                } else if th.Type == RMWsum {
                    loc += 12
                }
            } else if th.Code == Success {
                if th.Type == Read || th.Type == ReadNonInc {
                    loc += 4 * (int(th.Words) + 1)
                } else if th.Type == Write || th.Type == WriteNonInc {
                    loc += 4
                } else if th.Type == RMWbits || th.Type == RMWsum {
                    loc += 8
                }
            } else {
                return fmt.Errorf("Transaction code %x not implimented.\n\n%v\n\n%x", th.Code, p, data)
            }
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

func (t TranHeader) String() string {
    return fmt.Sprintf("loc: %d, v = %d, id = %d, %d words, type = %d, infocode = %v", t.Loc, t.Version, t.ID, t.Words, t.Type, t.Code)
}

func (t *TranHeader) Parse(data []byte, loc int) error {
	t.Loc = loc
	t.Version = uint8((data[loc] | 0xf0) >> 4)
	t.ID = (uint16(data[loc] & 0x0f) << 16)
	t.ID |= uint16(data[loc + 1])
	t.Words = uint8(data[loc + 2])
	t.Type = TypeID((data[loc + 3] & 0xf0) >> 4)
	if _, ok := goodtypeids[t.Type]; !ok {
        return fmt.Errorf("Invalid transaction type: %v, %x", t.Type, data[loc:loc+4])
	}
	t.Code = InfoCode((data[loc + 3] & 0x0f))
	if _, ok := goodcodes[t.Code]; !ok {
		return fmt.Errorf("Invalid Transaction code: %v", t.Code)
	}
	return error(nil)
}

func ResendPacket(id uint16) Packet {
    return Packet{Version: Version, ID: id, Type: Resend}
}

func StatusPacket() Packet {
    return Packet{Version: Version, ID: 0, Type: Status}
}

type TrafficHistory struct {
    Data uint8
    FailedCRC, Dropped bool
    Type EventType
}

func (th TrafficHistory) String() string {
    return fmt.Sprintf("hist: failed = %t, dropped = %t, type = %v", th.FailedCRC, th.Dropped, th.Type)
}

func (th *TrafficHistory) Encode() {
    th.Data = uint8(th.Type)
    if th.FailedCRC {
        th.Data |= (0x1 << 7)
    }
    if th.Dropped {
        th.Data |= (0x1 << 6)
    }
}

func (th *TrafficHistory) Parse() {
    th.FailedCRC = th.Data & (0x1 << 7) > 0
    th.Dropped = th.Data & (0x1 << 6) > 0
    th.Type = EventType(th.Data | 0x7)
}

func NewHistory(data uint8) *TrafficHistory {
    th := &TrafficHistory{Data: data}
    th.Parse()
    return th
}

type EventType uint8

var HardReset EventType = 0x0
var IPbusReset EventType = 0x1
var IPbusControlReq EventType = 0x2
var IPbusStatusReq EventType = 0x3
var IPbusResendReq EventType = 0x4
var UnrecognisedUDP EventType = 0x5
var ValidPing EventType = 0x6
var ValidARP EventType = 0x7
var OtherEthernet EventType = 0xf

type StatusResp struct {
    MTU, Buffers, Next uint32
    IncomingHistory []*TrafficHistory
    ReceivedHeaders, OutgoingHeaders []*PackHeader
}

func (sr StatusResp) String() string {
    s := fmt.Sprintf("%d byte MTU, %d buffers, next ID = %d.\n", sr.MTU, sr.Buffers, sr.Next)
    s += fmt.Sprintf("incoming: %v\n", sr.IncomingHistory)
    for _, p := range sr.ReceivedHeaders {
        s += fmt.Sprintf("recv: %v\n", p)
    }
    for _, p := range sr.OutgoingHeaders {
        s += fmt.Sprintf("sent: %v\n", p)
    }
    return s
}

func (s StatusResp) Encode() []byte {
    out := make([]byte, 0, 60)
    out = append(out, 0x20) // version
    out = append(out, 0x00) // packet ID
    out = append(out, 0x00) // more packet Id
    out = append(out, 0xf1) // byte order + type = 1
    fmt.Printf("After packet header: %d bytes.\n", len(out))
    for i := 0; i < 4; i++ {
        shift := uint32(3 - i)
        mask := uint32(0xff) << shift
        out = append(out, uint8((s.MTU & mask) >> shift))
    }
    fmt.Printf("After MTU: %d bytes.\n", len(out))
    for i := 0; i < 4; i++ {
        shift := uint32(3 - i)
        mask := uint32(0xff) << shift
        out = append(out, uint8((s.Buffers & mask) >> shift))
    }
    fmt.Printf("After Buffers: %d bytes.\n", len(out))
    for i := 0; i < 4; i++ {
        shift := uint32(3 - i)
        mask := uint32(0xff) << shift
        out = append(out, uint8((s.Next & mask) >> shift))
    }
    fmt.Printf("After NextID : %d bytes.\n", len(out))
    for i := 0; i < 16; i++ {
        s.IncomingHistory[i].Encode()
        out = append(out, s.IncomingHistory[i].Data)
    }
    fmt.Printf("After incoming traffic history: %d bytes.\n", len(out))
    for i := 0; i < 4; i++ {
        out = append(out, s.ReceivedHeaders[i].Encode()...)
    }
    fmt.Printf("After received packets: %d bytes.\n", len(out))
    for i := 0; i < 4; i++ {
        out = append(out, s.OutgoingHeaders[i].Encode()...)
    }
    fmt.Printf("After sent packets: %d bytes.\n", len(out))
    return out
}

func (s *StatusResp) Parse(data []byte) error {
    if len(data) != 64 {
        return fmt.Errorf("Status report requires 60 bytes, received %d.", len(data))
    }
    fmt.Printf("Parsing status: %x\n", data)
    head := &PackHeader{}
    head.Parse(data, 0, false)
    if head.ID != 0 || head.Type != Status {
        return fmt.Errorf("Failed to parse packet with ID = %d and type = %d as Status response.",
                          head.ID, head.Type)
    }
    loc := 4
    s.MTU = 0
    for i := 0; i < 4; i++ {
        //fmt.Printf("MTU byte %d [%d] = %X\n", i, loc + i, data[loc + i])
        s.MTU += uint32(data[loc + i]) << uint32((3 - i) * 8)
    }
    loc += 4
    s.Buffers = 0
    for i := 0; i < 4; i++ {
        s.Buffers += uint32(data[loc + i]) << uint32((3 - i) * 8)
    }
    loc += 4
    s.Next = 0
    s.Next += uint32(data[loc + 1]) << 8
    s.Next += uint32(data[loc + 2])
    fmt.Printf("s.Next = 0x%x = %d\n", s.Next, s.Next)
    loc += 4
    for i := 0; i < 16; i++ {
        s.IncomingHistory = append(s.IncomingHistory, NewHistory(uint8(data[loc + i])))
    }
    loc += 16
    for i := 0; i < 4; i++ {
        p := &PackHeader{}
        if data[loc] == 0 {
            continue
        }
        if err := p.Parse(data, loc, false); err != nil {
            return err
        }
        s.ReceivedHeaders = append(s.ReceivedHeaders, p)
        //fmt.Printf("Received headers = %v\n", s.ReceivedHeaders)
        loc += 4
    }
    for i := 0; i < 4; i++ {
        if data[loc] == 0 {
            continue
        }
        p := &PackHeader{}
        if err := p.Parse(data, loc, false); err != nil {
            return err
        }
        s.OutgoingHeaders = append(s.OutgoingHeaders, p)
        loc += 4
    }
    return error(nil)
}
