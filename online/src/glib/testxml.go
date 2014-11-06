package main

import (
    "data"
    "fmt"
    "glibxml"
)

func main() {
    mod, err := glibxml.Parse("addr_table/nicks_sc_daq.xml")
    if err != nil {
        panic(err)
    }
    fmt.Printf("%v\n", mod)
    for _, r := range mod.Registers {
        fmt.Printf("\t%v\n", r)
        for _, b := range r.Blocks {
            fmt.Printf("\t\t%v\n", b)
            for n, m := range b.Masks {
                fmt.Printf("\t\t\t%s mask = 0x%x\n", n, m)
            }
        }
    }
    for _, p := range mod.Ports {
        fmt.Printf("\t%v\n", p)
    }
    for _, m := range mod.Modules {
        fmt.Printf("\t%v\n", m)
        for _, r := range m.Registers {
            fmt.Printf("\t\t%v\n", r)
            for _, b := range r.Blocks {
                fmt.Printf("\t\t\t%v\n", b)
                for n, m := range b.Masks {
                    fmt.Printf("\t\t\t\t%s mask = 0x%x\n", n, m)
                }
            }
        }
        for _, p := range m.Ports {
            fmt.Printf("\t%v\n", p)
        }
        csrctrl := mod.Registers["csr"].Blocks["ctrl"]
        fmt.Printf("csr.ctrl at 0x%x = 0x%x\n", csrctrl.LAddress, csrctrl.GAddress)
        csrstat := mod.Registers["csr"].Blocks["stat"]
        fmt.Printf("csr.stat at 0x%x = 0x%x\n", csrstat.LAddress, csrstat.GAddress)
        tcsrchanctrl := mod.Modules["timing"].Registers["csr"].Blocks["chan_ctrl"]
        fmt.Printf("timing.csr.chan_ctrl at 0x%x = 0x%x\n",
                   tcsrchanctrl.LAddress, tcsrchanctrl.GAddress)
        fmt.Printf("timing.csr.chan_ctrl.phase mask = 0x%x\n", mod.Modules["timing"].Registers["csr"].Blocks["chan_ctrl"].Masks["phase"])
        rr := data.ReqResp{}
        vals := csrstat.GetReads(rr)
        fmt.Printf("vals = %v\n", vals)
    }
}
