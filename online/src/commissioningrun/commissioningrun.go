package main

import (
    "data"
    "glibxml"
    "flag"
    "fmt"
    "mail"
    "solid"
//    "net"
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
    dir := flag.String("dir", ".", "output directory")
    period := flag.Int("time", 10, "Length of run [s]")
    nruns := flag.Int("nrun", 1, "Number of runs to perform [-ve implies infinite].")
    store := flag.String("store", "", "Long term storage location.")
    allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
    passfile := flag.String("pass", "pass.txt", "Email password file.")
    flag.Parse()
    e := mail.E{}
    err := e.Load(*passfile)
    if err != nil {
        panic(err)
    }
    defer cleanexit(e)
    mod, err := glibxml.Parse("GLIB", "nicks_c.xml")
    if err != nil {
        panic(err)
    }
    runtime.GOMAXPROCS(4)
    fmt.Println("Solid's SM1 online DAQ software!")
    control := solid.New(*dir, *store)
    for i := 0; i < 1; i++ {
        control.AddFPGA(mod)
    }
    errp := control.Start()
    if errp.Err != nil {

        fmt.Printf("Error in Start(): %v\n", errp)
        e.Log("Error in start:", errp)
        return
    }
    irun := 0
    dt := time.Duration(*period) * time.Second
    for *nruns < 0 || irun < *nruns {
        r, err := data.NewRun(uint32(irun), "test", dt)
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
        irun += 1
        stop := time.Now()
        fmt.Printf("Stopped running at %v [%v]\n", stop, stop.Sub(r.Start))
    }
    fmt.Printf("Stopped all runs.\n")
    control.Quit()
}
