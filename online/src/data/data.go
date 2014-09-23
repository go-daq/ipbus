package data

import (
    "ipbus"
    "net"
    "time"
)

type Run struct{
    Num uint32
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
    RAddr net.UDPAddr
    RespIndex int
}

func (r *ReqResp) Encode() error {
    *Bytes = (*Bytes)[:0]
    enc, err := r.Out.Encode()
    if err != nil {
        return err
    }
    Bytes = append(enc)
    r.RespIndex = len(Bytes)
}

func (r *ReqResp) Decode() error {
    r.In = ipbus.PackHeader{}
    if err := r.In.Parse(&((*r.Bytes)[r.RespIndex:])); err != nil {
        return err
    }
}

func CreateReqRes(req ipbus.Packet) ReqRes {
    r := ReqResp{Out: req}
}
