package glibxml

import (
    "data"
    "fmt"
    "github.com/jteeuwen/go-pkg-xmlx"
    oldipbus "old/ipbus"
    "strconv"
    "strings"
)

func Parse(name, filename string) (Module, error) {
    basexmldoc := xmlx.New()
    err := basexmldoc.LoadFile(filename, nil)
    if err != nil {
        return Module{}, err
    }
    conns := basexmldoc.SelectNodes("", "connection")
    for _, conn := range conns {
        id := conn.As("", "id")
        if id == name {
            ip := conn.As("", "uri")
            ip = strings.Replace(ip, "ipbusudp-2.0://", "", 1)
            fn := conn.As("", "address_table")
            fn = strings.Replace(fn, "file://", "", 1)
            xmldoc := xmlx.New()
            err := xmldoc.LoadFile(fn, nil)
            if err != nil {
                return Module{}, err
            }
            modnode := xmldoc.SelectNode("", "module")
            return NewModule(name, ip, modnode)
        }
    }
    return Module{}, fmt.Errorf("XML file for connection '%s' not found in %s.", name, filename)
}

func NewModule(name, ip string, node *xmlx.Node) (Module, error) {
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
        reg, err := newregister(r, addr)
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
        submod, err := NewModule(name, ip, m)
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
    m := Module{name, ip, id, addr, regs, submods, ports}
    return m, nil
}

type Module struct {
    Name string
    IP string
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

func (p port) Read(size uint8, pack * oldipbus.Packet) {
    tr := oldipbus.MakeReadNonInc(size, p.GAddress)
    pack.Add(tr)
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

func newregister(node *xmlx.Node, modaddr uint32) (register, error) {
    id := node.As("", "id")
    addr := node.As("", "address")
    address, err := strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return register{}, err
    }
    descr := node.As("", "description")
    fwinfo := node.As("", "fwinfo")
    words := make(map[string]word)
    wordnodes := node.SelectNodes("", "word")
    for _, w := range wordnodes {
        word, err := newword(w, uint32(address) + modaddr)
        if err != nil {
            return register{}, err
        }
        words[word.ID] = word
    }
    ports := make(map[string]port)
    portnodes := node.SelectNodes("", "port")
    for _, p := range portnodes {
        port, err := newport(p, uint32(address) + modaddr)
        if err != nil {
            return register{}, err
        }
        ports[port.ID] = port
    }
    reg := register{id, uint32(address), uint32(address) + modaddr, descr,
                    fwinfo, words, ports}
    return reg, nil
}


type register struct {
    ID string
    LAddress, GAddress uint32
    Description, FWInfo string
    Words map[string]word
    Ports map[string]port
}

func (r register) String() string {
    s := fmt.Sprintf("Reg id = %s at 0x%x -> 0x%x", r.ID, r.LAddress, r.GAddress)
    if r.Description != "" {
        s += fmt.Sprintf(", %s", r.Description)
    }
    if r.FWInfo != "" {
        s += fmt.Sprintf(", %s", r.FWInfo)
    }
    s += fmt.Sprintf(", %d words", len(r.Words))
    return s
}

func (r register) Write(data []byte, pack *oldipbus.Packet) {
    tr := oldipbus.MakeWrite(r.GAddress, data)
    pack.Add(tr)
}

func (r register) Read(size uint8, pack * oldipbus.Packet) {
    tr := oldipbus.MakeRead(size, r.GAddress)
    pack.Add(tr)
}

func (r register) GetReads(reqresp data.ReqResp) [][]uint32 {
    return getReads(r.GAddress, reqresp)
}

func newword(node *xmlx.Node, regaddr uint32) (word, error) {
    id := node.As("", "id")
    addr := node.As("", "address")
    address, err := strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return word{}, err
    }
    masknodes := node.SelectNodes("", "mask")
    masks := make([]uint32, len(masknodes))
    maskindices := make(map[string]int)
    for i, m := range masknodes {
        mask, err := newmask(m)
        if err != nil {
            return word{}, err
        }
        maskindices[mask.ID] = i
        masks[i] = mask.Mask
    }
    w := word{id, uint32(address), uint32(address) + regaddr, maskindices, masks}
    return w, nil
}

type word struct {
    ID string
    LAddress, GAddress uint32
    MaskIndices map[string]int
    Masks []uint32
}

func (w word) String() string {
    return fmt.Sprintf("Word id = %s at 0x%x -> 0x%x, %d masks", w.ID,
                       w.LAddress, w.GAddress, len(w.Masks))
}

func (w word) Read(pack * oldipbus.Packet) {
    tr := oldipbus.MakeRead(1, w.GAddress)
    pack.Add(tr)
}

func (w word) ReadNonInc(size uint8, pack * oldipbus.Packet) {
    tr := oldipbus.MakeReadNonInc(1, w.GAddress)
    pack.Add(tr)
}

func (w word) GetReads(reqresp data.ReqResp) [][]uint32 {
    return getReads(w.GAddress, reqresp)
}

func (w word) GetMaskedReads(mask string, reqresp data.ReqResp) []uint32 {
    n, ok := w.MaskIndices[mask]
    if !ok {
        return []uint32{}
    }
    return w.GetMaskedReadsIndex(n, reqresp)
}

func (w word) GetMaskedReadsIndex(n int, reqresp data.ReqResp) []uint32 {
    if n >= len(w.Masks) {
        return []uint32{}
    }
    m := w.Masks[n]
    shift := uint32(0)
    for i := uint32(0); i < 32; i++ {
        if m & (0x1 << i) > 0 {
            shift = i
            break
        }
    }
    values := getReads(w.GAddress, reqresp)
    maskedvalues := []uint32{}
    for _, val := range values {
        mval := (val[0] & m) >> shift
        maskedvalues = append(maskedvalues, mval)
    }
    return maskedvalues
}

/*
Find all read transaction corresponding to a given global address. Parse the data
into a uint32 for each word and return a slice of them.
*/
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
    // requests from this memory word
    for itr, tr := range reqresp.Out.Transactions {
        if tr.Type == oldipbus.Read || tr.Type == oldipbus.ReadNonInc {
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

func (r word) Write(value uint32, pack * oldipbus.Packet) {
    data := []byte{0, 0, 0, 0}
    for i := uint(0); i < 4; i++ {
        shift := i * 8
        mask := uint32(0xff) << shift
        data[i] = uint8((value & mask) >> shift)
    }
    tr := oldipbus.MakeWrite(r.GAddress, data)
    pack.Add(tr)
}

func (w word) MaskedWrite(name string, value uint32, pack * oldipbus.Packet) error {
    n, ok := w.MaskIndices[name]
    if !ok {
        return fmt.Errorf("Fail to do masked read with unknown mask named %s", name)
    }
    return w.MaskedWriteIndex(n, value, pack)
}

func (w word) MaskedWriteIndex(n int, value uint32, pack *oldipbus.Packet) error {
    if n >= len(w.Masks) {
        return fmt.Errorf("Masked write failed, index %d out of range.", n)
    }
    mask := w.Masks[n]
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
    //fmt.Printf("Masked write of %s:%s, GAddress = 0x%x, and term = 0x%x, or = x%x.\n", w.ID, name, w.GAddress, x, y)
    tr := oldipbus.MakeRMWbits(w.GAddress, x, y)
    pack.Add(tr)
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
