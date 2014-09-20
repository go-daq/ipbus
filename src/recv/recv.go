package main

import(
    "fmt"
    "net"
    "time"
    "ipbus"
)

func main() {
    addr, err := net.ResolveUDPAddr("udp", "localhost:9989")
    if err != nil {
        panic(err)
    }
    fmt.Printf("local address = %v\n", addr)
    conn, err := net.ListenUDP("udp", addr)
    if err != nil {
        panic(err)
    }
    fakedata := make([]uint32, 0, 1024)
    for i := uint32(0); i < 1024; i++ {
        fakedata = append(fakedata, i)
    }
    data := make([]byte, 1024)
    fmt.Println("Waiting for data...")
    for {
        n, raddr, err := conn.ReadFrom(data)
        if err != nil {
            panic(err)
        }
        if n > 0 {
            fmt.Printf("Received %d bytes from %v: %x.\n", n, raddr, data[:n])
            p := ipbus.Packet{}
            err := p.Decode(data[:n])
            if err != nil {
                panic(err)
            }
            fmt.Printf("packet: %v\n", p)
            for _, t := range p.Transactions {
                if t.Type == ipbus.Read {
                    rp := ipbus.Packet{
                        Version: uint8(2),
                        ID: uint16(0),
                        Type: ipbus.Control,
                    }
                    reply := ipbus.MakeRead(t.Words, t.Body[0])
                    reply.Body = reply.Body[:0]
                    reply.Code = ipbus.Success
                    reply.Body = append(reply.Body, fakedata[:t.Words]...)
                    rp.Transactions = append(rp.Transactions, reply)
                    fmt.Printf("Sending packet: %v\n", rp)
                    outdata, err := rp.Encode()
                    if err != nil {
                        panic(err)
                    }
                    n, err := conn.WriteTo(outdata, raddr)
                    if err != nil {
                        panic(err)
                    }
                    fmt.Printf("Sent %d bytes.\n", n)
                }
            }
        }
        time.Sleep(100 * time.Millisecond)
    }
}
