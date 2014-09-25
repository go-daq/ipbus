package main

import(
    "flag"
    "net"
    "ipbus"
    "fmt"
    "runtime"
    "time"
    "os"
)

func send(loc string, packs chan ipbus.Packet, nread int) {
    pack := ipbus.Packet{}
    replydata := make([]byte, 65535)
    addr, err := net.ResolveUDPAddr("udp", loc)
    if err != nil {
        panic(err)
    }
    //fmt.Printf("Sending to %v\n", addr)
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        panic(err)
    }
    packet := ipbus.MakePacket(ipbus.Control)
    for nread > 0 {
        readsize := uint8(0)
        if nread < 256 {
            readsize = uint8(nread)
        } else {
            readsize = uint8(255)
        }
        nread -= int(readsize)
        //fmt.Printf("Transaction reading %d words.\n", readsize)
        rt := ipbus.MakeRead(readsize, 0x003f01)
        packet.Transactions = append(packet.Transactions, rt)
    }
    //fmt.Printf("packet: %v\n", packet)
    data, err := packet.Encode()
    if err != nil {
        panic(err)
    }
    msg := 0
    for {
        //fmt.Printf("Sending %d: %x\n", msg, data)
        msg += 1
        _, err = conn.Write(data)
        if err != nil {
            panic(err)
        }
        //fmt.Printf("Sent %d bytes over UDP to %v.\n", n, addr)
        nreplybytes, _, err := conn.ReadFrom(replydata)
        if err != nil {
            panic(err)
        }
        if nreplybytes > 0 {
            //fmt.Printf("Received %d bytes from %v: %x.\n", n, raddr, replydata[:n])
            err := pack.Decode(replydata[:nreplybytes])
            if err != nil {
                panic(err)
            }
            packs <- pack
        }
    }
}

func main() {
    runtime.GOMAXPROCS(2)
    totalbytes := int64(0)
    totalpacks := 0
    packs := make(chan ipbus.Packet, 100)
    nread := flag.Int("n", 200, "number of bytes")
    fn := flag.String("fn", "test.dat", "output filename")
    rt := flag.Int("t", 1, "seconds to run")
    flag.Parse()
    //fmt.Printf("Received: %x\n", replydata[:nreplybytes])
    outp, err := os.Create(*fn)
    if err != nil{
        panic(err)
    }
    start := time.Now()
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("localhost:%d", 9988 + i)
        go send(loc, packs, *nread)
    }
    to := time.After(time.Duration(*rt) * time.Second)
    running := true
    stop := time.Now()
    for running {
        select {
        case <- to:
            running = false
            stop = time.Now()
        case p := <-packs:
            totalpacks += 1
            totalbytes += int64(4)
            for _, t := range p.Transactions {
                totalbytes += int64(4)
                totalbytes += int64(len(t.Body))
                if _, err := outp.Write(t.Body); err != nil {
                    panic(err)
                }
            }
            //fmt.Printf("totalbytes = %d (+%d)\n", totalbytes, n)
        }
    }
    if err := outp.Close(); err != nil {
        panic(err)
    }
    end := time.Now()
    dt := end.Sub(start)
    nbytes := float64(totalbytes)
    fmt.Printf("Whole transaction sent %d bytes. Took %v.\n", totalbytes, end.Sub(start))
    rate := nbytes / dt.Seconds() / 1000000.0
    fmt.Printf("start to end rate = %v bytes in %v = %v MB/s\n", nbytes, dt, rate)
    fmt.Printf("Received %d packets.\n", totalpacks)
    fmt.Printf("End %v after stop.\n", end.Sub(stop))
}
