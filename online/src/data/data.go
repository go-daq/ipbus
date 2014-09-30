package data

import (
    //"fmt"
    "ipbus"
    "net"
    "strconv"
    "time"
)

type Run struct{
    Num uint32
    Name string
    Start, End time.Time
}

type Config struct{
    Vs, Ts []float64
    Last time.Time
}

type ReqResp struct{
    Out ipbus.Packet
    In ipbus.PackHeader
    Bytes []byte
    Sent, Received time.Time
    RAddr net.Addr
    RespIndex, RespSize int
}

func (r ReqResp) Encode() ([]byte, error) {
    out := make([]byte, 0, len(r.Bytes) + 32)
    /* Write my header: 
        remote IP - 32 bit
        port (16 bit), length (16 bit) 
        time sent - 64 bit
        time received - 64 bit
    */
    host, port, err := net.SplitHostPort(r.RAddr.String())
    if err != nil {
        return []byte{}, err
    }
    ip := net.ParseIP(host)
    ipv4 := []byte(ip[12:])
    out = append(out, ipv4...)
    p, err := strconv.ParseUint(port, 10, 16)
    if err != nil {
        return []byte{}, err
    }
    out = append(out, uint8((p & 0xff00) >> 8))
    out = append(out, uint8(p & 0x00ff))
    words := uint16(len(r.Bytes) / 4) + 6
    out = append(out, uint8((words & 0xff00) >> 8))
    out = append(out, uint8((words & 0x00ff)))
    sentnano := r.Sent.UnixNano()
    for i := 0; i < 8; i++ {
        shift := uint((7 - i) * 8)
        mask := int64(0xff << shift)
        out = append(out, uint8((sentnano & mask) >> shift))
    }
    recvnano := r.Received.UnixNano()
    for i := 0; i < 8; i++ {
        shift := uint((7 - i) * 8)
        mask := int64(0xff << shift)
        out = append(out, uint8((recvnano & mask) >> shift))
    }
    out = append(out, r.Bytes...)
    return out, error(nil)
}

func (r *ReqResp) EncodeOut() error {
    r.Bytes = r.Bytes[:0]
    enc, err := r.Out.Encode()
    if err != nil {
        return err
    }
    r.Bytes = append(r.Bytes, enc...)
    r.RespIndex = len(r.Bytes)
    for i := 0; i < 1500; i++ {
        r.Bytes = append(r.Bytes, 0x0)
    }
    return error(nil)
}

func (r *ReqResp) Decode() error {
    //fmt.Printf("Decoding from loc = %d, %d bytes\n", r.RespIndex, len(r.Bytes))
    //fmt.Println("Decode done.")
    r.In = ipbus.PackHeader{}
    if err := r.In.Parse(r.Bytes, r.RespIndex, false); err != nil {
        return err
    }
    if r.In.Type != ipbus.Status {
        if err := r.In.Parse(r.Bytes, r.RespIndex, true); err != nil {
            return err
        }
    }
    return error(nil)

}

func CreateReqResp(req ipbus.Packet) ReqResp {
    return ReqResp{Out: req}
}

