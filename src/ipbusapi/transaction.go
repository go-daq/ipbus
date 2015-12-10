package ipbusapi

// Response to a control transaction
type Response struct {
    Err error
    Code infoCode
    Data []uint32
    DataB []byte
}

type transaction struct {
    Type typeID
    NWords uint8
    Addr uint32
    Input []uint32
    resp chan Response
    byteslice, closechan bool
}

type packet struct {
    transactions []transaction
    reqcapacity, respcapacity uint
    reqspace, repspace uint
    request []byte
}

func emptypacket(pt packetType) packet {
    trans := make([]transaction, 0, 8)
    request := make([]byte, 4, 1472)
    request[0] = protocolversion << 4
    request[3] = 0xf0 & uint8(pt)

    // Normal IP packet has up to 1500 bytes. IP header is 20 bytes, UDP 
    // header is 8 bytes. This leaves 368 words for the ipbus data.
    // First word is the packet header, so to he 
    size := (MaxPacketSize - 28) / 4
    return packet{trans, size, size, 0, 0, request} // For normal packet 
}

func (p *packet) add(trans transaction) error {
    return error(nil)
}

func (p *packet) setid(id uint16) {
    part := uint8(id & 0xff)
    p.request[2] = part
    part = uint8(id >> 8)
    p.request[1] = part
}
