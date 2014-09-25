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
    dir := flag.String("dir", ".", "output directory")
    flag.Parse()
    runtime.GOMAXPROCS(4)
    fmt.Println("Solid's SM1 online DAQ software!")
    con := solid.New(*dir)
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
    start := time.Now()
    fmt.Printf("Going to run for %v at %v.\n", dt, start)
    con.Run("test", dt)
    stop := time.Now()
    fmt.Printf("Stopped run at %v [%v]\n", stop, stop.Sub(start))
    con.Quit()
}
