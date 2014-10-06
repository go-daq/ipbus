package main

import (
    "data"
    "mail"
    "flag"
    "fmt"
    "solid"
    "net"
    "runtime"
    "time"
)

func cleanexit(e mail.E) {
    if r := recover(); r != nil {
        if err, ok := r.(error); ok {
            ep := data.MakeErrPack(err)
            fmt.Printf("Caught a panic: %v\n", ep)
            subject := fmt.Sprintf("Online DAQ crash at %v", time.Now())
            msg := fmt.Sprintf("Caught a panic: %v\n", ep)
            if err := e.Send(subject, msg); err != nil {
                fmt.Println(err)
            }
        } else {
            panic(r)
        }
    }
    fmt.Println("Clean exit.")
}


func main() {
    addr := flag.String("addr", "localhost", "remote adress")
    dir := flag.String("dir", ".", "output directory")
    period := flag.Int("time", 10, "Length of run [s]")
    allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
    passfile := flag.String("pass", "pass.txt", "Email password file.")
    flag.Parse()
    e := mail.E{}
    err := e.Load(*passfile)
    if err != nil {
        panic(err)
    }
    defer cleanexit(e)
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
    errp := control.Start()
    if errp.Err != nil {

        fmt.Printf("Error in Start(): %v\n", errp)
        e.Log("Error in start:", errp)
        return
    }
    dt := time.Duration(*period) * time.Second
    r, err := data.NewRun(0, "test", dt)
    if err != nil {
        panic(err)
    }
    if r.Commit.Modified && !(*allowmod) {
        panic(fmt.Errorf("Code has local modifications: %v\n", r.Commit))
    }
    errp = control.Run(r)
    if errp.Err != nil {
        fmt.Printf("Error in run: %v\n", errp)
        e.Log("Error in run:", errp)
    }
    stop := time.Now()
    fmt.Printf("Stopped run at %v [%v]\n", stop, stop.Sub(r.Start))
    control.Quit()
}
