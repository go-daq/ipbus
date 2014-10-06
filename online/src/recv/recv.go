package main

import(
    "flag"
    "fmt"
    "net"
    "time"
    "ipbus"
)

func listen(loc string, drop bool) {
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
    order := []byte{0x00, 0x00, 0x00, 0x12, 0x89, 0xab, 0xcd, 0xef}
    for i := 0; i < 1024; i++ {
        fakedata = append(fakedata, order[i % len(order)])
    }
    data := make([]byte, 10024)
    fmt.Println("Waiting for data...")
    received := make([]ipbus.Packet, 0, 4)
    sent := make([]ipbus.Packet, 0, 4)
    next := uint32(1)
    lastskip := time.Now().Add(-time.Hour)
    lastdrop := time.Now().Add(-time.Hour)
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
            if drop && p.ID == uint16(2337) {
                now := time.Now()
                if now.Sub(lastskip) > time.Minute {
                    fmt.Printf("Deliberately skipping: %v\n", p)
                    lastskip = now
                    continue
                } else {
                    fmt.Printf("Not skipping: %v\n", p)
                }
            }
            if p.Type == ipbus.Control {
                if uint32(p.ID) < next {
                    fmt.Printf("WARNING: Got ID = %d, expected %d, would be dropped.\n", p.ID, next)
                }
                if next == 65535 {
                    next = uint32(1)
                } else {
                    next = uint32(p.ID + 1)
                }
                received = append(received, p)
                if len(received) > 4 {
                    received = received[len(received) - 4:]
                }
            }
            if p.Type == ipbus.Status {
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
                    sp := &ipbus.PackHeader{}
                    if len(sent) > i {
                        sp.Version = sent[i].Version
                        sp.ID = sent[i].ID
                        sp.Type = sent[i].Type
                    }
                    resp.OutgoingHeaders = append(resp.OutgoingHeaders, sp)
                    rp := &ipbus.PackHeader{}
                    if len(received) > i {
                        rp.Version = received[i].Version
                        rp.ID = received[i].ID
                        rp.Type = received[i].Type
                    }
                    resp.ReceivedHeaders = append(resp.ReceivedHeaders, rp)
                }
                outdata = resp.Encode()
                fmt.Printf("%s: Received a status request. Replying with:%v\n", loc, resp)
                /*
                sent = append(sent, resp)
                if len(sent) > 4 {
                    sent = sent[len(sent) - 4:]
                }
                */
                fmt.Printf("Sending %d bytes.\n", len(outdata))
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
                sent = append(sent, rp)
                if len(sent) > 4 {
                    sent = sent[len(sent) - 4:]
                }
            }
            //fmt.Printf("Sending packet: %v, %x\n", rp, outdata)
            now := time.Now()
            if p.ID != uint16(1333) || !drop || now.Sub(lastdrop) < time.Minute {
                _, err = conn.WriteTo(outdata, raddr)
                if err != nil {
                    panic(err)
                }
            } else {
                fmt.Printf("Deliberately dropping reply: %v\n", p)
                lastdrop = now
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
    period := flag.Int("time", 600, "run time [s]")
    drop := flag.Bool("drop", false, "Drop sent or received packets.")
    flag.Parse()
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("%s:%d", *addr, 9988 + i)
        go listen(loc, *drop)
    }
    dt := time.Duration(*period) * time.Second
    time.Sleep(dt)
}
