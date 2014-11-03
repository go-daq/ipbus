package main

import (
    "github.com/jteeuwen/go-pkg-xmlx"
    "fmt"
    "strconv"
    "strings"
)


type block struct {
    id string
    address uint32
    description string
    fwinfo string
    mode string
    registers []register
}

func (b *block) Parse(node *xmlx.Node) (int, error) {
    nread := 1
    b.id = node.As("", "id")
    addr := node.As("", "address")
    address, err :=  strconv.ParseUint(addr, 0, 32)
    if err != nil {
        return nread, err
    }
    b.address = uint32(address)
    decendants := make([]*xmlx.Node, 0)
    if node.HasAttr("", "module") {
        fn := node.As("", "module")
        fn = strings.Replace(fn, "file://", "addr_table/", 1)
        fmt.Printf("Reading register %s from other file: %s\n", b.id, fn)
        otherdoc := xmlx.New()
        err = otherdoc.LoadFile(fn, nil)
        if err != nil {
            return nread, err
        }
        othernodes := otherdoc.SelectNodesRecursive("", "node")
        node = othernodes[0]
        fmt.Printf("othernodes = [\n%v\n]\n", othernodes)
        decendants = node.SelectNodesRecursive("", "node")
        decendants = decendants[1:]
        fmt.Printf("decendants = [\n%v\n]\n", othernodes)
    } else {
        decendants = node.SelectNodesRecursive("", "node")
        nread += len(decendants) - 1
    }
    idec := 1
    for idec < len(decendants) {
        regnode := decendants[idec]
        rid := regnode.As("", "id")
        raddr := regnode.As("", "address")
        fmt.Printf("\treg id = %s, addr = %s\n", rid, raddr)
        raddress, err := strconv.ParseUint(raddr, 0, 32)
        if err != nil {
            return nread, err
        }
        reg := register{rid, uint32(raddress), []value{}}
        valuenodes := regnode.SelectNodesRecursive("", "node")
        fmt.Printf("%d value nodes.\n", len(valuenodes))
        idec += len(valuenodes)
        for ival := 1; ival < len(valuenodes); ival++ {
            vid := valuenodes[ival].As("", "id")
            vm := valuenodes[ival].As("", "mask")
            fmt.Printf("\t\tval id = %s, mask = %s\n", vid, vm)
            vmask, err := strconv.ParseUint(vm, 0, 32)
            if err != nil {
                return nread, err
            }
            val := value{vid, uint32(vmask)}
            reg.values = append(reg.values, val)
        }
        b.registers = append(b.registers, reg)
    }
    return nread, nil
}

func (b block) String() string {
    s := fmt.Sprintf("Block, ID = %s at 0x%x", b.id, b.address)
    if b.description != "" {
        s += fmt.Sprintf(", %s", b.description)
    }
    if b.fwinfo != "" {
        s += fmt.Sprintf(", %s", b.fwinfo)
    }
    if len(b.registers) > 0 {
        s += fmt.Sprintf(", %d registers", len(b.registers))
    }
    return s
}

type register struct {
    id string
    address uint32
    values []value
}
func (r register) String() string {
    s := fmt.Sprintf("reg ID = %s at 0x%x", r.id, r.address)
    if len(r.values) > 0 {
        s += fmt.Sprintf(", %d values", len(r.values))
    }
    return s
}

type value struct {
    id string
    mask uint32
}

func (v value) String() string {
    return fmt.Sprintf("value ID = %s, mask = 0x%x", v.id, v.mask)
}

func main() {
    xmldoc := xmlx.New()
    err := xmldoc.LoadFile("addr_table/nicks_sc_daq.xml", nil)
    if err != nil {
        panic(err)
    }
    xmlnodes := xmldoc.SelectNodesRecursive("", "node")
    inode := 1
    nnodes := len(xmlnodes)
    for inode < nnodes {
        xmlblock := xmlnodes[inode]
        b := block{}
        nread, err := b.Parse(xmlblock)
        if err != nil {
            panic(err)
        }
        fmt.Printf("Read %d nodes.\n", nread)
        inode += nread
        fmt.Printf("%v\n", b)
        for _, r := range b.registers {
            fmt.Printf("\t%v\n", r)
            for _, v := range r.values {
                fmt.Printf("\t\t%v\n", v)
            }
        }
        /*
        id := xmlblock.As("", "id")
        addr := xmlblock.As("", "address")
        address, err := strconv.ParseUint(addr, 0, 32)
        if err != nil {
            panic(err)
        }
        if xmlblock.HasAttr("", "module") {
            fn := xmlblock.As("", "module")
            fn = strings.Replace(fn, "file://", "addr_table/", 1)
            fmt.Printf("Reading register %s from other file: %s\n", id, fn)
            otherdoc := xmlx.New()
            err = otherdoc.LoadFile(fn, nil)
            if err != nil {
                panic(err)
            }
            othernodes := otherdoc.SelectNodesRecursive("", "node")
            //fmt.Printf("othernodes = [%v\n]\n", othernodes)
            xmlblock = othernodes[0]
            //fmt.Printf("xmlblock = [%v\n]\n", xmlblock)
        }
        //fmt.Printf("xmlblock = [%v\n]\n", xmlblock)
        descr := xmlblock.As("", "description")
        info := xmlblock.As("", "fwinfo")
        bl := block{id, uint32(address), descr, info, []register{}}

        blockdescendants := xmlblock.SelectNodesRecursive("", "node")
        nsubnodes := len(blockdescendants)
        inode += nsubnodes
        isubnode := 1
        for isubnode < nsubnodes {
            xmlregister := blockdescendants[isubnode]
            rid := xmlregister.As("", "id")
            raddr := xmlregister.As("", "address")
            raddress, err := strconv.ParseUint(raddr, 0, 32)
            if err != nil {
                panic(err)
            }
            reg := register{rid, uint32(raddress), []value{}}
            registerdescendants := xmlregister.SelectNodesRecursive("", "node")
            nsubsubnodes := len(registerdescendants)
            isubnode += nsubsubnodes
            if nsubsubnodes > 1 {
                for isubsubnode := 1; isubsubnode < nsubsubnodes; isubsubnode++ {
                    vid := registerdescendants[isubsubnode].As("", "id")
                    vm := registerdescendants[isubsubnode].As("", "mask")
                    vmask, err := strconv.ParseUint(vm, 0, 32)
                    if err != nil {
                        fmt.Printf("Failed to convert %s from %s\n", vm, vid)
                        fmt.Printf("This is a value from the %s register.\n", reg.id)
                        fmt.Printf("This is in the %s block.\n", bl.id)
                        fmt.Printf("%v\n", registerdescendants[isubsubnode])
                        panic(err)
                    }
                    val := value{vid, uint32(vmask)}
                    reg.values = append(reg.values, val)
                }
            }
            bl.registers = append(bl.registers, reg)
        }
        fmt.Printf("%v\n", bl)
        for _, reg := range bl.registers {
            fmt.Printf("\t%v\n", reg)
            for _, val := range reg.values {
                fmt.Printf("\t\t%v\n", val)
            }
        }
    */
    }
}
