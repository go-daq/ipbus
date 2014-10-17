package main

import (
    "fmt"
    "ipbus"
    "net"
    "time"
)

func read(conn * net.UDPConn) {
    buf := make([]byte, 10024)
    for {
        n, addr, err := conn.ReadFrom(buf)
        if err != nil {
            panic(err)
        }
        if n > 0 {
            fmt.Printf("received %x from %v\n", buf[:n], addr)
        }
        s := ipbus.StatusResp{}
        err = s.Parse(buf[:n])
        fmt.Printf("Received status response: %v\n", s)
        if err != nil {
            panic(err)
        }
    }
}

func main() {
    addr, err := net.ResolveUDPAddr("udp", "192.168.200.3:50001")
    if err != nil {
        panic(err)
    }
    fmt.Printf("Attempting to send to %v\n", addr)
    conn, err := net.DialUDP("udp", nil, addr)
    if err != nil {
        panic(err)
    }
    go read(conn)
    pack := ipbus.StatusPacket()
    data, err := pack.Encode()
    if err != nil {
        panic(err)
    }
    fmt.Printf("Sending %x\n", data)
    _, err = conn.Write(data)
    if err != nil {
        panic(err)
    }
    time.Sleep(10 * time.Second)
} 
