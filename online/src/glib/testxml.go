package main

import (
    "fmt"
    "glibxml"
)
/*
import (
    "github.com/jteeuwen/go-pkg-xmlx"
    "fmt"
    "strconv"
    "strings"
)

func parse(filename string) (module, error) {
    xmldoc := xmlx.New()
    err := xmldoc.LoadFile(filename, nil)
    if err != nil {
        return module{}, err
    }
    modnode := xmldoc.SelectNode("", "module")
    return newmodule(modnode)
}

func newmodule(node *xmlx.Node) (module, error) {
    id := node.As("", "id")
    addr := uint32(0)
    if node.HasAttr("", "address") {
        saddr := node.As("", "address")
        fmt.Printf("Module node has an address: %s\n", saddr)
        address, err := strconv.ParseUint(saddr, 0, 32)
        if err != nil {
            return module{}, nil
        }
        addr = uint32(address)
    }
    if node.HasAttr("", "module") {
        fn := node.As("", "module")
        fn = strings.Replace(fn, "file://", "addr_table/", 1)
        fmt.Printf("Need to read module from another file: %s\n", fn)
        otherdoc := xmlx.New()
        err := otherdoc.LoadFile(fn, nil)
        if err != nil {
            return module{}, err
        }
        node = otherdoc.SelectNode("", "module")
    }
    regnodes := node.SelectNodes("", "register")
    regs := make(map[string]register)
    for _, r := range regnodes {
        reg, err := newreg(r, addr)
        if err != nil {
            return module{}, err
        }
        regs[reg.id] = reg
    }
    submodnodes := node.SelectNodesRecursive("", "module")
    fmt.Printf("Found %d sub-module nodes.\n", len(submodnodes))
    submods := make(map[string]module)
    for _, m := range submodnodes[1:] {
        fmt.Printf("Need to also parse sub-module: %s\n", m)
        submod, err := newmodule(m)
        if err != nil {
            return module{}, err
        }
        submods[submod.id] = submod
    }
    portnodes := node.SelectNodes("", "port")
    ports := make(map[string]port)
    for _, p := range portnodes {
        port, err := newport(p, addr)
        if err != nil {
            return module{}, err
        }
        ports[port.id] = port
    }
    m := module{id, addr, regs, submods, ports}
    return m, nil
}

type module struct {
    id string
    address uint32
    registers map[string]register
    modules map[string]module
    ports map[string]port
}

func (m module) String() string {
    s := fmt.Sprintf("mod ID = %s at 0x%x, %d regs, %d mods, %d ports", m.id,
                     m.address, len(m.registers), len(m.modules), len(m.ports))
    return s
}

func newport(node *xmlx.Node, modaddr uint32) (port, error) {
    id := node.As("", "id")
    addr := node.As("", "address")
    address, err := strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return port{}, err
    }
    descr := node.As("", "description")
    fwinfo := node.As("", "fwinfo")
    p := port{id, uint32(address), uint32(address) + modaddr, descr, fwinfo}
    return p, nil
}

type port struct {
    id string
    laddress, gaddress uint32
    description, fwinfo string
}

func (p port) String() string {
    s := fmt.Sprintf("port ID = %s at 0x%x -> 0x%x", p.id, p.laddress, p.gaddress)
    if p.description != "" {
        s += fmt.Sprintf(", %s", p.description)
    }
    if p.fwinfo != "" {
        s += fmt.Sprintf(", %s", p.fwinfo)
    }
    return s
}

func newreg(node *xmlx.Node, modaddr uint32) (register, error) {
    id := node.As("", "id")
    addr := node.As("", "address")
    address, err := strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return register{}, err
    }
    descr := node.As("", "description")
    fwinfo := node.As("", "fwinfo")
    blocks := make(map[string]block)
    blocknodes := node.SelectNodes("", "block")
    for _, b := range blocknodes {
        block, err := newblock(b, uint32(address) + modaddr)
        if err != nil {
            return register{}, err
        }
        blocks[block.id] = block
    }
    reg := register{id, uint32(address), uint32(address) + modaddr, descr,
                    fwinfo, blocks}
    return reg, nil
}


type register struct {
    id string
    laddress, gaddress uint32
    description, fwinfo string
    blocks map[string]block

}

func (r register) String() string {
    s := fmt.Sprintf("Reg id = %s at 0x%x -> 0x%x", r.id, r.laddress, r.gaddress)
    if r.description != "" {
        s += fmt.Sprintf(", %s", r.description)
    }
    if r.fwinfo != "" {
        s += fmt.Sprintf(", %s", r.fwinfo)
    }
    s += fmt.Sprintf(", %d blocks", len(r.blocks))
    return s
}

func newblock(node *xmlx.Node, regaddr uint32) (block, error) {
    id := node.As("", "id")
    addr := node.As("", "address")
    address, err := strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return block{}, err
    }
    masks := make(map[string]uint32)
    masknodes := node.SelectNodes("", "mask")
    for _, m := range masknodes {
        mask, err := newmask(m)
        if err != nil {
            return block{}, err
        }
        masks[mask.id] = mask.mask
    }
    b := block{id, uint32(address), uint32(address) + regaddr, masks}
    return b, nil
}

type block struct {
    id string
    laddress, gaddress uint32
    masks map[string]uint32
}

func (b block) String() string {
    return fmt.Sprintf("Block id = %s at 0x%x -> 0x%x, %d masks", b.id,
                       b.laddress, b.gaddress, len(b.masks))
}

func newmask(node *xmlx.Node) (mask, error) {
    id := node.As("", "id")
    mk := node.As("", "mask")
    msk, err := strconv.ParseUint(mk, 0, 32)
    if err != nil {
        return mask{}, err
    }
    return mask{id, uint32(msk)}, nil
}

type mask struct {
    id string
    mask uint32
}

func (m mask) String() string {
    return fmt.Sprintf("Mask id = %s, mask = 0x%x", m.id, m.mask)
}
*/

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
    }
}
