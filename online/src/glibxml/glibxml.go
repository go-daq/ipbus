package glibxml

import (
    "data"
    "fmt"
    "github.com/jteeuwen/go-pkg-xmlx"
    "ipbus"
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

func (p port) Read(size uint8, pack * ipbus.Packet) {
    tr := ipbus.MakeReadNonInc(size, p.GAddress)
    pack.Transactions = append(pack.Transactions, tr)
}

func (p port) GetReads(rr data.ReqResp) [][]uint32 {
    return getReads(p.GAddress, rr)
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

func (r register) Write(data []byte, pack *ipbus.Packet) {
    tr := ipbus.MakeWrite(r.GAddress, data)
    pack.Transactions = append(pack.Transactions, tr)
}

func (r register) Read(size uint8, pack * ipbus.Packet) {
    tr := ipbus.MakeRead(size, r.GAddress)
    pack.Transactions = append(pack.Transactions, tr)
}

func (r register) GetReads(reqresp data.ReqResp) [][]uint32 {
    return getReads(r.GAddress, reqresp)
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

func (b block) Read(pack * ipbus.Packet) {
    tr := ipbus.MakeRead(1, b.GAddress)
    pack.Transactions = append(pack.Transactions, tr)
}

func (b block) ReadNonInc(size uint8, pack * ipbus.Packet) {
    tr := ipbus.MakeReadNonInc(1, b.GAddress)
    pack.Transactions = append(pack.Transactions, tr)
}

/*
Find all read transaction corresponding to a given global address. Parse the data
into a uint32 for each block and return a slice of them.
*/
func (b block) GetReads(reqresp data.ReqResp) [][]uint32 {
    return getReads(b.GAddress, reqresp)
}

func (b block) GetMaskedReads(mask string, reqresp data.ReqResp) []uint32 {
    m, ok := b.Masks[mask]
    if !ok {
        return []uint32{}
    }
    shift := uint32(0)
    for i := uint32(0); i < 32; i++ {
        if m & (0x1 << i) > 0 {
            shift = i
            break
        }
    }
    values := getReads(b.GAddress, reqresp)
    maskedvalues := []uint32{}
    for _, val := range values {
        mval := (val[0] & m) >> shift
        maskedvalues = append(maskedvalues, mval)
    }
    return maskedvalues
}

func getReads(address uint32, reqresp data.ReqResp) [][]uint32 {
    addr := []byte{0, 0, 0, 0}
    for i := uint(0); i < 4; i++ {
        shift := 24 - i * 8
        mask := uint32(0xff) << shift
        addr[i] = uint8(address & mask >> shift)
    }
    values := [][]uint32{}
    locations := []int{}
    sizes := []int{}
    // Find the locations of the reply transactions corresponding to read 
    // requests from this memory block
    for itr, tr := range reqresp.Out.Transactions {
        if tr.Type == ipbus.Read || tr.Type == ipbus.ReadNonInc {
            correctaddr := true
            for i := 0; i < 4; i++ {
                if addr[i] != tr.Body[i] {
                    correctaddr = false
                    break
                }
            }
            if correctaddr {
                reptrans := reqresp.In.Trans[itr]
                locations = append(locations, reptrans.Loc)
                sizes = append(sizes, int(reptrans.Words))
            }
        }
    }
    // Convert the responses into words and store in slices.
    for i, loc := range locations {
        loc += 4 // Skip the header
        size := sizes[i]
        values = append(values, []uint32{})
        for iword := 0; iword < size; iword++ {
            val := uint32(0)
            for ibyte := 0; ibyte < 4; ibyte++ {
                byteindex := loc + 4 * iword + ibyte
                b := reqresp.Bytes[byteindex]
                val += uint32(uint32(b) << uint32(24 - ibyte * 8))
            }
            values[i] = append(values[i], val)
        }
    }
    return values
}

/*
func (b block) GetReadNonInc(size uint8, reqresp data.ReqResp) ([]uint32, error) {
    addr := []byte{0, 0, 0, 0}
    for i := uint32(0); i < 4; i++ {
        shift := i * 8
        mask := uint32(0xff) << shift
        addr[i] = uint8(b.GAddress & mask >> shift)
    }
    value = make([]uint32, 0, size)
    fmt.Printf("address = 0x%x\n", addr)
    for _, tr := range reqresp.In.Trans {
        if tr.Type == ipbus.Read {
            correctaddr := true
            for i := 0; i < 4; i++ {
                if addr[i] != reqresp.Bytes[tr.Loc + 4 + i] {
                    correctaddr = false
                    break
                }
            }
            if correctaddr {
                for iw := uint8(0); iw < size; iw++ {
                    val := uint32(0)
                    for i := uint(0); i < 4 * size; i++ {
                        index := tr.Loc + (iw + 1) * 4 // header then data
                        val += uint32(reqresp.Bytes[tr.Loc + (iw + 1) * 4 + i] << (i * 8))
                    }
                }
                return val, nil
            }
        }
    }
    return uint32(0), fmt.Errorf("Read transaction for %s not found.", b.ID)
}
*/

func (b block) Write(value uint32, pack * ipbus.Packet) {
    data := []byte{0, 0, 0, 0}
    for i := uint(0); i < 4; i++ {
        shift := i * 8
        mask := uint32(0xff) << shift
        data[i] = uint8((value & mask) >> shift)
    }
    tr := ipbus.MakeWrite(b.GAddress, data)
    pack.Transactions = append(pack.Transactions, tr)
}

func (b block) MaskedWrite(name string, value uint32, pack * ipbus.Packet) error {
    mask, ok := b.Masks[name]
    if !ok {
        return fmt.Errorf("MakedWrite failed, unknown mask: %s", name)
    }
    shift := uint32(0)
    for i := uint32(0); i < 32; i++ {
        if mask & (0x1 << i) > 0 {
            shift = i
            break
        }
    }
    value = value << shift
    x := uint32(0xffffffff) & value
    x = x | (0xffffffff ^ mask)
    y := value
    tr := ipbus.MakeRMWbits(b.GAddress, x, y)
    pack.Transactions = append(pack.Transactions, tr)
    return nil
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
