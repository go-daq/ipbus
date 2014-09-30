package main

import(
    "flag"
    "fmt"
    "net"
    "time"
    "ipbus"
)

func listen(loc string) {
    addr, err := net.ResolveUDPAddr("udp", loc)
    if err != nil {
        panic(err)
    }
    fmt.Printf("local address = %v\n", addr)
    conn, err := net.ListenUDP("udp", addr)
    if err != nil {
        panic(err)
    }
    fakedata := make([]byte, 0, 1024)
    order := []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
    for i := 0; i < 1024; i++ {
        fakedata = append(fakedata, order[i % len(order)])
    }
    data := make([]byte, 10024)
    fmt.Println("Waiting for data...")
    for {
        n, raddr, err := conn.ReadFrom(data)
        if err != nil {
            panic(err)
        }
        nt := 0
        if n > 0 {
            //fmt.Printf("Received %d bytes from %v: %x.\n", n, raddr, data[:n])
            outdata := []byte{}
            p := ipbus.Packet{}
            err := p.Decode(data[:n])
            if err != nil {
                panic(err)
            }
            if p.Type == ipbus.Status {
                next := uint32(12)
                rp := ipbus.StatusPacket()
                outdata, err := rp.Encode()
                if err != nil {
                    panic(err)
                }
                resp := ipbus.StatusResp{
                    MTU: 1500,
                    Buffers: 2,
                    Next: next,
                }
                hist := &ipbus.TrafficHistory{
                    Data: uint8(0x2),
                    FailedCRC: false,
                    Dropped: false,
                    Type: ipbus.IPbusControlReq,
                }
                for i := 0; i < 16; i++ {
                    resp.IncomingHistory = append(resp.IncomingHistory, hist)
                }
                for i := 0; i < 4; i++ {
                    nid := uint16(12 - 4 + i)
                    p := &ipbus.PackHeader{
                        Version: ipbus.Version,
                        ID: nid,
                        Type: ipbus.Control,
                    }
                    resp.ReceivedHeaders = append(resp.ReceivedHeaders, p)
                    resp.OutgoingHeaders = append(resp.OutgoingHeaders, p)
                }
                moredata := resp.Encode()
                outdata = append(outdata, moredata...)
            } else {
            //fmt.Printf("packet from %v with ID = %d\n", raddr, p.ID)
                rp := ipbus.MakePacket(ipbus.Control)
                rp.ID = p.ID
                for _, t := range p.Transactions {
                    if t.Type == ipbus.Read {
                        //fmt.Printf("Read transaction requesting %d words from %x [%v].\n", t.Words, t.Body, t)
                        nt += 1
                        //time.Sleep(10 * time.Millisecond)
                        reply := ipbus.MakeReadReply(fakedata[:4 * int(t.Words)])
                        //fmt.Printf("reply = %v\n", reply)
                        rp.Add(reply)
                    } else if t.Type == ipbus.Write {
                        nt += 1
                        reply := ipbus.MakeWriteReply(t.Words)
                        rp.Add(reply)
                    }
                }
                outdata, err = rp.Encode()
                if err != nil {
                    panic(err)
                }
            }
            //fmt.Printf("Sending packet: %v, %x\n", rp, outdata)
            _, err = conn.WriteTo(outdata, raddr)
            if err != nil {
                panic(err)
            }
            //fmt.Printf("Sent ID = %d, %d bytes to %v.\n", rp.ID, n, raddr)
            //fmt.Printf("Received %d transactions.\n", nt)
            //fmt.Printf("Sent %v\n", outdata)
        }
        //time.Sleep(10 * time.Minute)
    }
}

func main() {
    addr := flag.String("addr", "localhost", "local address")
    flag.Parse()
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("%s:%d", *addr, 9988 + i)
        go listen(loc)
    }
    time.Sleep(600 * time.Second)
}
