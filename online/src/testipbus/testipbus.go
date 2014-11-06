package main

import (
    "data"
    "flag"
    "fmt"
    "glibxml"
    "ipbus"
    "mail"
    "net"
    "runtime"
    "solid"
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
    ipaddr := flag.String("addr", "localhost", "remote adress")
    dir := flag.String("dir", ".", "output directory")
    //period := flag.Int("time", 10, "Length of run [s]")
    //allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
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
    mod, err := glibxml.Parse("addr_table/nicks_sc_daq.xml")
    if err != nil {
        panic(err)
    }
    control := solid.New(*dir)
    loc := fmt.Sprintf("%s:%d", *ipaddr, 9988)
    addr, err := net.ResolveUDPAddr("udp", loc)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Adding FPGA at %v\n", addr)
    control.AddFPGA(addr)
    errp := control.Start()
    if errp.Err != nil {
        fmt.Printf("Error in Start(): %v\n", errp)
        e.Log("Error in start:", errp)
        return
    }
    p := ipbus.MakePacket(ipbus.Control)
    mod.Registers["id"].Blocks["magic"].Read(&p)
    mod.Registers["csr"].Blocks["ctrl"].Read(&p)
    mod.Registers["csr"].Read(3, &p)
    mod.Registers["csr"].Blocks["ctrl"].Read(&p)
    mod.Ports["chan"].Read(64, &p)
    mod.Ports["chan"].Read(22, &p)
    fmt.Printf("Created read transaction using glib map: %v\n", p)
    replies := make(chan data.ReqResp)
    control.Send(0, p, replies)
    rep := <-replies
    fmt.Printf("Got reply: %v\n", rep)
    fmt.Printf("rep.Out: %d transactions\n", len(rep.Out.Transactions))
    fmt.Printf("rep.In: %d transactions\n", len(rep.In.Trans))
    fmt.Printf("Attempting to get reads of id.magic block.\n")
    values := mod.Registers["id"].Blocks["magic"].GetReads(rep)
    fmt.Printf("id.magic values = %x\n", values)
    fmt.Printf("Attempting to get reads of csr register.\n")
    values = mod.Registers["csr"].GetReads(rep)
    fmt.Printf("csr values = %x\n", values)
    fmt.Printf("Attempting to get masked reads of csr.ctrl.chan_sel.\n")
    mvals := mod.Registers["csr"].Blocks["ctrl"].GetMaskedReads("chan_sel", rep)
    fmt.Printf("csr.ctrl.chan_sel values = %x\n", mvals)
    fmt.Printf("Attempting to get reads of chan port.\n")
    values = mod.Ports["chan"].GetReads(rep)
    fmt.Printf("chan values = %x\n", values)
    for _, v := range values {
        fmt.Printf("%d values\n", len(v))
    }
}
