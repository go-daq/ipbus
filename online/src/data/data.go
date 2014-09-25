package data

import (
    "fmt"
    "ipbus"
    "net"
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
    Bytes *[]byte
    Sent, Received time.Time
    RAddr net.Addr
    RespIndex, RespSize int
}

func (r *ReqResp) Encode() error {
    *r.Bytes = (*r.Bytes)[:0]
    enc, err := r.Out.Encode()
    if err != nil {
        return err
    }
    *r.Bytes = append(*r.Bytes, enc...)
    r.RespIndex = len(*r.Bytes)
    for i := 0; i < 1500; i++ {
        *r.Bytes = append(*r.Bytes, 0x0)
    }
    return error(nil)
}

func (r *ReqResp) Decode() error {
    fmt.Printf("Decoding from loc = %d, %d bytes\n", r.RespIndex, len(*r.Bytes))
    fmt.Println("Decode done.")
    r.In = ipbus.PackHeader{}
    err := r.In.Parse(r.Bytes, r.RespIndex)
    return err
}

func CreateReqResp(req ipbus.Packet) ReqResp {
    return ReqResp{Out: req}
}
