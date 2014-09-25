package main

import (
    "flag"
    "fmt"
    "solid"
    "net"
    "runtime"
    "time"
)

func main() {
    addr := flag.String("addr", "localhost", "remote adress")
    flag.Parse()
    runtime.GOMAXPROCS(2)
    fmt.Println("Solid's SM1 online DAQ software!")
    con := solid.New()
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("%s:%d", *addr, 9988 + i)
        addr, err := net.ResolveUDPAddr("udp", loc)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Adding FPGA at %v\n", addr)
        con.AddFPGA(addr)
    }
    con.Start()
    dt := 10 * time.Second
    fmt.Printf("Going to run for %v.\n", dt)
    con.Run("test", dt)
    con.Quit()
}
