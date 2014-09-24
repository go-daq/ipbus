package main

import(
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
    order := []byte{0x11, 0x22, 0x33, 0x44}
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
            p := ipbus.Packet{}
            err := p.Decode(data[:n])
            if err != nil {
                panic(err)
            }
            //fmt.Printf("packet: %v\n", p)
            rp := ipbus.MakePacket(ipbus.Control)
            for _, t := range p.Transactions {
                if t.Type == ipbus.Read {
                    //fmt.Printf("Read transaction requesting %d words from %x [%v].\n", t.Words, t.Body, t)
                    nt += 1
                    reply := ipbus.MakeReadReply(fakedata[:4 * int(t.Words)])
                    //fmt.Printf("reply = %v\n", reply)
                    rp.Transactions = append(rp.Transactions, reply)
                }
            }
            //fmt.Printf("Sending packet: %v\n", rp)
            outdata, err := rp.Encode()
            if err != nil {
                panic(err)
            }
            _, err = conn.WriteTo(outdata, raddr)
            if err != nil {
                panic(err)
            }
            //fmt.Printf("Sent %d bytes.\n", n)
            //fmt.Printf("Received %d transactions.\n", nt)
            //fmt.Printf("Sent %v\n", outdata)
        }
        //time.Sleep(10 * time.Minute)
    }
}

func main() {
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("localhost:%d", 9988 + i)
        go listen(loc)
    }
    time.Sleep(120 * time.Second)
}
