package main

import (
    "fmt"
    "os"
    "time"
    "ipbus"
)

func main() {
    fakedata := make([]uint32, 0, 1024)
    for i := uint32(0); i < 1024; i++ {
        fakedata = append(fakedata, i)
    }
    rp := ipbus.Packet{
        Version: uint8(2),
        ID: uint16(0),
        Type: ipbus.Control,
    }
    reply := ipbus.MakeRead(200, 0)
    reply.Body = reply.Body[:0]
    reply.Code = ipbus.Success
    reply.Body = append(reply.Body, fakedata[:200]...)
    rp.Transactions = append(rp.Transactions, reply)
    fmt.Printf("Sending packet: %v\n", rp)
    outdata, err := rp.Encode()
    if err != nil {
        panic(err)
    }
    nbytes := len(outdata)
    outp, err := os.Create("test.dat")
    if err != nil {
        panic(err)
    }
    start := time.Now()
    n, err := outp.Write(outdata)
    end := time.Now()
    if err != nil {
        panic(err)
    }
    if n != nbytes {
        fmt.Printf("WARNING: %d of %d bytes written.\n", n, nbytes)
    }
    dt := end.Sub(start).Seconds()
    rate := float64(nbytes) / dt / 1000000.0
    fmt.Printf("Wrote %d bytes in %v. rate = %f MB/s\n", nbytes, end.Sub(start), rate)
    if err := outp.Close(); err != nil {
        panic(err)
    }
}
