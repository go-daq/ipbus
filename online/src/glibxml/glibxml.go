package glibxml

import (
    "github.com/jteeuwen/go-pkg-xmlx"
    "fmt"
    "strconv"
    "strings"
)

func Parse(filename string) (Module, error) {
    xmldoc := xmlx.New()
    err := xmldoc.LoadFile(filename, nil)
    if err != nil {
        return Module{}, err
    }
    modnode := xmldoc.SelectNode("", "module")
    return NewModule(modnode)
}

func NewModule(node *xmlx.Node) (Module, error) {
    id := node.As("", "id")
    addr := uint32(0)
    if node.HasAttr("", "address") {
        saddr := node.As("", "address")
        fmt.Printf("Module node has an address: %s\n", saddr)
        address, err := strconv.ParseUint(saddr, 0, 32)
        if err != nil {
            return Module{}, nil
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
            return Module{}, err
        }
        node = otherdoc.SelectNode("", "module")
    }
    regnodes := node.SelectNodes("", "register")
    regs := make(map[string]register)
    for _, r := range regnodes {
        reg, err := newreg(r, addr)
        if err != nil {
            return Module{}, err
        }
        regs[reg.ID] = reg
    }
    submodnodes := node.SelectNodesRecursive("", "module")
    fmt.Printf("Found %d sub-module nodes.\n", len(submodnodes))
    submods := make(map[string]Module)
    for _, m := range submodnodes[1:] {
        fmt.Printf("Need to also parse sub-module: %s\n", m)
        submod, err := NewModule(m)
        if err != nil {
            return Module{}, err
        }
        submods[submod.ID] = submod
    }
    portnodes := node.SelectNodes("", "port")
    ports := make(map[string]port)
    for _, p := range portnodes {
        port, err := newport(p, addr)
        if err != nil {
            return Module{}, err
        }
        ports[port.ID] = port
    }
    m := Module{id, addr, regs, submods, ports}
    return m, nil
}

type Module struct {
    ID string
    Address uint32
    Registers map[string]register
    Modules map[string]Module
    Ports map[string]port
}

func (m Module) String() string {
    s := fmt.Sprintf("mod ID = %s at 0x%x, %d regs, %d mods, %d ports", m.ID,
                     m.Address, len(m.Registers), len(m.Modules), len(m.Ports))
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
    ID string
    LAddress, GAddress uint32
    Description, FWInfo string
}

func (p port) String() string {
    s := fmt.Sprintf("port ID = %s at 0x%x -> 0x%x", p.ID, p.LAddress, p.GAddress)
    if p.Description != "" {
        s += fmt.Sprintf(", %s", p.Description)
    }
    if p.FWInfo != "" {
        s += fmt.Sprintf(", %s", p.FWInfo)
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
        blocks[block.ID] = block
    }
    reg := register{id, uint32(address), uint32(address) + modaddr, descr,
                    fwinfo, blocks}
    return reg, nil
}


type register struct {
    ID string
    LAddress, GAddress uint32
    Description, FWInfo string
    Blocks map[string]block

}

func (r register) String() string {
    s := fmt.Sprintf("Reg id = %s at 0x%x -> 0x%x", r.ID, r.LAddress, r.GAddress)
    if r.Description != "" {
        s += fmt.Sprintf(", %s", r.Description)
    }
    if r.FWInfo != "" {
        s += fmt.Sprintf(", %s", r.FWInfo)
    }
    s += fmt.Sprintf(", %d blocks", len(r.Blocks))
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
        masks[mask.ID] = mask.Mask
    }
    b := block{id, uint32(address), uint32(address) + regaddr, masks}
    return b, nil
}

type block struct {
    ID string
    LAddress, GAddress uint32
    Masks map[string]uint32
}

func (b block) String() string {
    return fmt.Sprintf("Block id = %s at 0x%x -> 0x%x, %d masks", b.ID,
                       b.LAddress, b.GAddress, len(b.Masks))
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
    ID string
    Mask uint32
}

func (m mask) String() string {
    return fmt.Sprintf("Mask id = %s, mask = 0x%x", m.ID, m.Mask)
}

/*
func main() {
    mod, err := parse("addr_table/nicks_sc_daq.xml")
    if err != nil {
        panic(err)
    }
    fmt.Printf("%v\n", mod)
    for _, r := range mod.registers {
        fmt.Printf("\t%v\n", r)
        for _, b := range r.blocks {
            fmt.Printf("\t\t%v\n", b)
            for n, m := range b.masks {
                fmt.Printf("\t\t\t%s mask = 0x%x\n", n, m)
            }
        }
    }
    for _, p := range mod.ports {
        fmt.Printf("\t%v\n", p)
    }
    for _, m := range mod.modules {
        fmt.Printf("\t%v\n", m)
        for _, r := range m.registers {
            fmt.Printf("\t\t%v\n", r)
            for _, b := range r.blocks {
                fmt.Printf("\t\t\t%v\n", b)
                for n, m := range b.masks {
                    fmt.Printf("\t\t\t\t%s mask = 0x%x\n", n, m)
                }
            }
        }
        for _, p := range m.ports {
            fmt.Printf("\t%v\n", p)
        }
        csrctrl := mod.registers["csr"].blocks["ctrl"]
        fmt.Printf("csr.ctrl at 0x%x = 0x%x\n", csrctrl.laddress, csrctrl.gaddress)
        csrstat := mod.registers["csr"].blocks["stat"]
        fmt.Printf("csr.stat at 0x%x = 0x%x\n", csrstat.laddress, csrstat.gaddress)
        tcsrchanctrl := mod.modules["timing"].registers["csr"].blocks["chan_ctrl"]
        fmt.Printf("timing.csr.chan_ctrl at 0x%x = 0x%x\n",
                   tcsrchanctrl.laddress, tcsrchanctrl.gaddress)
        fmt.Printf("timing.csr.chan_ctrl.phase mask = 0x%x\n", mod.modules["timing"].registers["csr"].blocks["chan_ctrl"].masks["phase"])
    }
}
*/
