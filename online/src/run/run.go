package main

import (
    "crash"
    "data"
    "glibxml"
    "flag"
    "fmt"
    "mail"
    "os/user"
    "solid"
    "strings"
    "runtime"
    "time"
)

func main() {
    dir := flag.String("dir", ".", "output directory")
    name := flag.String("name", "testing", "part of output filename")
    nuke := flag.Bool("nuke", false, "nuke FPGAs")
    coincidence := flag.Bool("coincidence", false, "require vertical/horizonatal coincidence to trigger.")
    duration := flag.Int("duration", 30, "Length o run [s]")
    threshold := flag.Int("threshold", -1, "Trigger threshold [ADC count above pedestal")
    muthreshold := flag.Int("muthreshold", 400, "Muon panel trigger threshold [ADC count above pedestal")
    randrate := flag.Float64("randrate", -1.0, "Random trigger rate [Hz]")
    nchans := flag.Int("nchans", 76, "Number of channels per GLIB.")
    nruns := flag.Int("nrun", 1, "Number of runs to perform [-ve implies infinite].")
    store := flag.String("store", "", "Long term storage location.")
    glibs := flag.String("glib", "GLIB1,GLIB2,GLIB3,GLIB4,GLIB5", "Comma separated string of GLIB module names (e.g. 'GLIB1,GLIB2,GLIB5')")
    allowmod := flag.Bool("allowmod", false, "Allow running even if code modified.")
    passfile := flag.String("pass", "pass.txt", "Email password file.")
    flag.Parse()
    modallowed := *allowmod
    if modallowed {
        currentuser, err := user.Current()
        fmt.Printf("Allowing run with modifications requested by %v [%s]\n", currentuser, currentuser.Username)
        if err != nil {
            modallowed = false
            fmt.Printf("Unable to get current user, cannot allow modifications.\n")
        } else {
            if currentuser.Username != "ryder" {
                modallowed = false
                fmt.Printf("Only Nick Ryder can run with local modifications.\n")
            }
        }
    }
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
    exit := crash.Exit{}
    exit.Add(crash.Log{})
    e := mail.E{}
    err := e.Load(*passfile)
    if err != nil {
        panic(err)
    }
    exit.Add(e)
    defer exit.CleanExit("main()")
    //defer cleanexit(e)
    modnames := strings.Split(*glibs, ",")
    mods := []glibxml.Module{}
    internaltrigger := false
    for _, modname := range modnames {
        if modname == "GLIB6" {
            fmt.Printf("GLIB6 in run, using internal triggers.\n")
            internaltrigger = true
        }
        mod, err := glibxml.Parse(modname, "c_triggered.xml")
        if err != nil {
            panic(err)
        }
        fmt.Printf("Parsed module %v. Registers:\n", mod)
        for name, reg := range mod.Registers {
            fmt.Printf("    %s: %v\n", name, reg)
            for wordname, word := range reg.Words {
                fmt.Printf("        word %s: %v\n", wordname, word)
            }
            for portname, port := range reg.Ports {
                fmt.Printf("        port %s: %v\n", portname, port)
            }
        }
        mods = append(mods, mod)
    }
    runtime.GOMAXPROCS(6)
    fmt.Println("Solid's SM1 online DAQ software!")
    control := solid.New(*dir, *store, channels, &exit, internaltrigger, *nuke)
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
    dt := time.Duration(*duration) * time.Second
    for *nruns < 0 || irun < *nruns {
        fmt.Printf("Making %dth run.\n", irun)
        fn := "triggered_"
        if *threshold < 0 {
            fn += fmt.Sprintf("random%0.3fHz_", *randrate)
        } else {
            fn += fmt.Sprintf("thr%d_", *threshold)
        }
        if *coincidence {
            fn += "coinc_"
        } else {
            fn += "nocoinc_"
        }
        fn += *name
        r, err := data.NewRun(uint32(irun), fn, dt, *threshold, *muthreshold, *randrate, *coincidence)
        if err != nil {
            panic(err)
        }
        if r.Commit.Modified && !(modallowed) {
            panic(fmt.Errorf("Code has local modifications: %v\n", r.Commit))
        }
        quit, errp := control.Run(r)
        if errp.Err != nil {
            fmt.Printf("Error in run: %v\n", errp)
            e.Log("Error in run:", errp)
        }
        irun += 1
        stop := time.Now()
        fmt.Printf("Stopped running at %v [%v]\n", stop, stop.Sub(r.Start))
        if quit {
            break
        }
    }
    fmt.Printf("Stopped all runs.\n")
    control.Quit()
}
