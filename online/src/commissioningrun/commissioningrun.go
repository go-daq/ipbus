package main

import (
    "data"
    "glibxml"
    "flag"
    "fmt"
    "mail"
    "solid"
    "strings"
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
    nchans := flag.Int("nchans", 38, "Number of channels per GLIB.")
    nruns := flag.Int("nrun", 1, "Number of runs to perform [-ve implies infinite].")
    store := flag.String("store", "", "Long term storage location.")
    glibs := flag.String("glib", "GLIB", "Comma separated string of GLIB module names (e.g. 'GLIB1,GLIB2,GLIB5')")
    allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
    passfile := flag.String("pass", "pass.txt", "Email password file.")
    flag.Parse()
    if *nchans != 38 && *nchans != 76 {
        panic(fmt.Errorf("Cannot have %d GLIB channels. Must be 38 or 76.", *nchans))
    }
    channels := make([]uint32, 0, 76)
    for ichan := uint32(0); ichan < 8; ichan++ {
        channels = append(channels, ichan)
    }
    for ichan := uint32(10); ichan < 18; ichan++ {
        channels = append(channels, ichan)
    }
    for ichan := uint32(19); ichan < 27; ichan++ {
        channels = append(channels, ichan)
    }
    for ichan := uint32(29); ichan < 37; ichan++ {
        channels = append(channels, ichan)
    }
    if *nchans == 76 {
        for ichan := uint32(38); ichan < 46; ichan++ {
            channels = append(channels, ichan)
        }
        for ichan := uint32(48); ichan < 56; ichan++ {
            channels = append(channels, ichan)
        }
        for ichan := uint32(57); ichan < 65; ichan++ {
            channels = append(channels, ichan)
        }
        for ichan := uint32(67); ichan < 75; ichan++ {
            channels = append(channels, ichan)
        }
    }
    e := mail.E{}
    err := e.Load(*passfile)
    if err != nil {
        panic(err)
    }
    defer cleanexit(e)
    modnames := strings.Split(*glibs, ",")
    mods := []glibxml.Module{}
    for _, modname := range modnames {
        mod, err := glibxml.Parse(modname, "nicks_c.xml")
        if err != nil {
            panic(err)
        }
        mods = append(mods, mod)
    }
    runtime.GOMAXPROCS(4)
    fmt.Println("Solid's SM1 online DAQ software!")
    control := solid.New(*dir, *store, channels)
    for _, mod := range mods {
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
        fmt.Printf("Making %dth run.\n", irun)
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
