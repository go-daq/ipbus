package main

import(
    "net"
    "ipbus"
    "fmt"
    "time"
)

func main() {
    addr, err := net.ResolveUDPAddr("udp", "localhost:9989")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Sending to %v\n", addr)
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        panic(err)
    }
    start := time.Now()
    packet := ipbus.Packet{
        Version: uint8(2),
        ID: uint16(0),
        Type: ipbus.Control,
    }
    readsize := uint8(200)
    rt := ipbus.MakeRead(readsize, 0x003f01)
    packet.Transactions = append(packet.Transactions, rt)
    fmt.Printf("packet: %v\n", packet)
    data, err := packet.Encode()
    if err != nil {
        panic(err)
    }
    n, err := conn.Write(data)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Sent %d bytes over UDP.\n", n)
    replydata := make([]byte, 1024)
    n, _, err = conn.ReadFrom(replydata)
    if err != nil {
        panic(err)
    }
    received := time.Now()
    decoded := time.Now()
    if n > 0 {
        //fmt.Printf("Received %d bytes from %v: %x.\n", n, raddr, replydata[:n])
        received = time.Now()
        p := ipbus.Packet{}
        err := p.Decode(replydata[:n])
        decoded = time.Now()
        if err != nil {
            panic(err)
        }
        //fmt.Printf("packet: %v\n", p)
    }
    conn.Close()
    end := time.Now()
    fmt.Printf("Whole transaction sent %d words. Took %v.\n", readsize, end.Sub(start))
    fmt.Printf("%v since receiving data.\n", end.Sub(received))
    dt := end.Sub(start)
    nbytes := float64(readsize) * 4.0
    rate := nbytes / dt.Seconds() / 1000000.0
    fmt.Printf("start to end rate = %v bytes in %v = %v MB/s\n", nbytes, dt, rate)
    dt = decoded.Sub(received)
    rate = nbytes / dt.Seconds() / 1000000.0
    fmt.Printf("decode rate = %v bytes in %v = %v MB/s\n", nbytes, dt, rate)
}
