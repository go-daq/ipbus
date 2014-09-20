package ipbus

import (
    "encoding/binary"
    "bytes"
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

var goodcodes map[InfoCode]bool = map[InfoCode]bool {
    Success : true,
    BadHeader : true,
    BusReadError : true,
    BusWriteError : true,
    BusReadTimeout : true,
    BusWriteTimeout : true,
    Request : true,
}

type TypeID uint8

var Read TypeID = 0x0
var Write TypeID = 0x1
var ReadNonInc TypeID = 0x2
var WriteNonInc TypeID = 0x3
var RMWbits TypeID = 0x4
var RMWsum TypeID = 0x5

var goodtypeids map[TypeID]bool = map[TypeID]bool {
    Read : true,
    Write : true,
    ReadNonInc : true,
    WriteNonInc : true,
    RMWbits : true,
    RMWsum : true,
}

type Transaction struct {
    Version uint8
    ID uint16
    Words uint8
    Type TypeID
    Code InfoCode
    Body []uint32
}

func (t Transaction) Encode() ([]byte, error) {
    data := []byte{}
    header := uint32(t.Code)
    header |= (uint32(t.Type) << 4)
    header |= (uint32(t.Words) << 8)
    header |= (uint32(t.ID) << 16)
    header |= (uint32(t.Version) << 28)
    buf := new(bytes.Buffer)
    if err := binary.Write(buf, binary.BigEndian, header); err != nil {
        return data, err
    }
    for _, w := range t.Body {
        if err := binary.Write(buf, binary.BigEndian, w); err != nil {
            return data, err
        }
    }
    return buf.Bytes(), error(nil)
}

func (t * Transaction) DecodeHeader(header uint32) error {
    v := uint8((header & 0xf0000000) >> 28)
    id := uint16((header & 0x0fff0000) >> 16)
    words := uint8((header & 0x0000ff00) >> 8)
    typeid := TypeID((header & 0x000000f0) >> 4)
    if _, ok := goodtypeids[typeid]; !ok {
        return errors.New("Invalid TypeID.")
    }
    code := InfoCode(header & 0x0000000f)
    if _, ok := goodcodes[code]; !ok {
        return errors.New("Invalid InfoCode.")
    }
    t.Version = v
    t.ID = id
    t.Words = words
    t.Type = typeid
    t.Code = code
    return error(nil)
}

func (t * Transaction) Decode(data []uint32) error {
    if len(data) == 0 {
        return errors.New("Empty data, cannot parse as Transaction")
    }
    header := data[0]
    if err := t.DecodeHeader(header); err != nil {
        return err
    }
    t.Body = data[1:]
    return error(nil)
}

func MakeRead(size uint8, addr uint32) Transaction {
    return Transaction{Version, 0, size, Read, Request, []uint32{addr}}
}

func MakeReadNonInc(size uint8, addr uint32) Transaction {
    return Transaction{Version, 0, size, ReadNonInc, Request, []uint32{addr}}
}

func MakeWrite(addr uint32, data []uint32) Transaction {
    body := make([]uint32, 0, len(data) + 1)
    body = append(body, addr)
    body = append(body, data...)
    return Transaction{Version, 0, uint8(len(data)), Write, Request, body}
}

func MakeWriteNonInc(addr uint32, data []uint32) Transaction {
    body := make([]uint32, 0, len(data) + 1)
    body = append(body, addr)
    body = append(body, data...)
    return Transaction{Version, 0, uint8(len(data)), WriteNonInc, Request, body}
}

func MakeRMWbits(addr uint32, and uint32, or uint32) Transaction {
    body := []uint32{addr, and, or}
    return Transaction{Version, 0, 1, RMWbits, Request, body}
}

func MakeRMWsum(addr uint32, addend uint32) Transaction {
    body := []uint32{addr, addend}
    return Transaction{Version, 0, 1, RMWsum, Request, body}
}

type PacketType uint8

const (
    Control PacketType = 0x0
    Status PacketType = 0x1
    Resend PacketType = 0x2
)

var goodpackettypes map[PacketType]bool  = map[PacketType]bool {
    Control : true,
    Status : true,
    Resend : true,
}

type Packet struct {
    Version uint8
    ID uint16
    Type PacketType
    Transactions []Transaction
}

func (p Packet) Encode() ([]byte, error) {
    data := []byte{}
    header := uint32(p.Type)
    header |= (uint32(0xf) << 4)
    header |= (uint32(p.ID) << 8)
    header |= (uint32(p.Version) << 28)
    buf := new(bytes.Buffer)
    if err := binary.Write(buf, binary.BigEndian, header); err != nil {
        return data, err
    }
    for _, t := range p.Transactions {
        d, err := t.Encode()
        if err != nil {
            return data, err
        }
        nwritten := 0
        for nwritten < len(d) {
            n, err := buf.Write(d[nwritten:])
            if err != nil {
                return data, err
            }
            nwritten += n
        }
    }
    return buf.Bytes(), error(nil)
}

func (p * Packet) Decode(data []byte) error {
    if len(data) < 4 {
        return errors.New("Empty data, cannot decode packet.")
    }
    val := uint32(0)
    buf := bytes.NewReader(data)
    if err := binary.Read(buf, binary.BigEndian, &val); err != nil {
        return err
    }
    p.Version = uint8((val & 0xf0000000) >> 28)
    p.ID = uint16((val & 0x00ffff00) >> 8)
    bo := uint8((val & 0x000000f0) >> 4)
    if bo != 0xf {
        return fmt.Errorf("Incorrect byte order: 0x%x", bo)
    }
    p.Type = PacketType((val & 0x0000000f))
    if _, ok := goodpackettypes[p.Type]; !ok {
        return fmt.Errorf("Invalid packet type: %v", p.Type)
    }
    //fmt.Printf("received header: %v\nReading words from %d bytes.\n", p, len(data) - 4)
    nword := 1
    for nword < len(data) / 4 {
        //fmt.Printf("nword = %d, len(data) = %d\n", nword, len(data))
        // Store the transaction header in val
        if err := binary.Read(buf, binary.BigEndian, &val); err != nil {
            return err
        }
        //fmt.Printf("transaction header = %x\n", val)
        nword += 1
        t := Transaction{}
        if err := t.DecodeHeader(val); err != nil {
            return err
        }
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
                nbody = int(t.Words)
            } else if t.Type == Write || t.Type == WriteNonInc {
                nbody = 0
            } else if t.Type == RMWbits {
                nbody = 1
            } else if t.Type == RMWsum {
                nbody = 1
            }
        } else {
            panic(errors.New("transaction code not implement properly."))
        }
        //fmt.Printf("Going to read %d words for body.\n", nbody)
        for i := 0; i < nbody; i++ {
            if err := binary.Read(buf, binary.BigEndian, &val); err != nil {
                return err
            }
            //fmt.Printf("Read val = %x\n", val)
            t.Body = append(t.Body, val)
            nword += 1
        }
        p.Transactions = append(p.Transactions, t)
        //fmt.Printf("Added transaction: %v\n", p)
        //fmt.Printf("end of loop: nword = %d, len(data) = %d\n", nword, len(data))
    }
    return error(nil)
}
