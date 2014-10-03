package main

import (
    "data"
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
    period := flag.Int("time", 10, "Length of run [s]")
    allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
    flag.Parse()
    runtime.GOMAXPROCS(4)
    fmt.Println("Solid's SM1 online DAQ software!")
    control := solid.New(*dir)
    for i := 0; i < 5; i++ {
        loc := fmt.Sprintf("%s:%d", *addr, 9988 + i)
        addr, err := net.ResolveUDPAddr("udp", loc)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Adding FPGA at %v\n", addr)
        control.AddFPGA(addr)
    }
    control.Start()
    dt := time.Duration(*period) * time.Second
    r, err := data.NewRun(0, "test", dt)
    if err != nil {
        panic(err)
    }
    if r.Commit.Modified && !(*allowmod) {
        panic(data.NotCommittedError)
    }
    control.Run(r)
    stop := time.Now()
    fmt.Printf("Stopped run at %v [%v]\n", stop, stop.Sub(r.Start))
    control.Quit()
}
