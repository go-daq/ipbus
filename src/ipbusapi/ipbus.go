// Package ipbusapi enables communication with FPGAs using the IPbus UDP protocol.
package ipbusapi

import (
    "encoding/binary"
)

// Supported IPbus protocol version
const IPbusVersion = 2.0

// Information codes
type InfoCode uint8

const BadHeader InfoCode = 0x1
const BusReadError InfoCode = 0x4
const BusWriteError InfoCode = 0x5
const BusReadTimeout InfoCode = 0x6
const BusWriteTimeout InfoCode = 0x7
const Request InfoCode = 0xf

// Transaction types
type TypeID uint8

const Read TypeID = 0x0
const Write TypeID = 0x1
const ReadNonInc TypeID = 0x2
const WriteNonInc TypeID = 0x3
const RMWbits TypeID = 0x4
const RMWsum TypeID = 0x5

func byte2uint32(bs []byte, order binary.ByteOrder) uint32 {
    return order.Uint32(bs))
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
