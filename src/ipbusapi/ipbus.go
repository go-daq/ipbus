// Package ipbusapi enables communication with FPGAs using the IPbus UDP protocol.
package ipbusapi

import (
    "encoding/binary"
)

// Supported IPbus protocol version
const IPbusVersion = 2.0
const protocolversion = uint8(2)

// Maxiumum Ethernet packet size (bytes)
var MaxPacketSize = uint(1500)

// Information codes
type infoCode uint8

const badHeader infoCode = 0x1
const busReadError infoCode = 0x4
const busWriteError infoCode = 0x5
const busReadTimeout infoCode = 0x6
const busWriteTimeout infoCode = 0x7
const request infoCode = 0xf

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
