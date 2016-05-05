package ipbus

import (
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type connectionset struct {
	Conns []connection `xml:"connection"`
}

type connection struct {
	Id      string `xml:"id,attr"`
	URI     string `xml:"uri,attr"`
	Address string `xml:"address_table,attr"`
}

type node struct {
	Id          string `xml:"id,attr"`
	Addr        string `xml:"address,attr"`
	Module      string `xml:"module,atr"`
	Description string `xml:"description,attr"`
	FWInfo      string `xml:"fwinfo,attr"`
	Mode        string `xml:"mode,attr"`
	Mask        string `xml:"mask,attr"`
	Nodes       []node
}

type nodes struct {
	Nodes []node
}

type block struct {
	id          string
	address     uint32
	description string
	fwinfo      string
	mode        string
}

func (b *block) register() Register {
	masks := make(map[string]uint32)
	noninc := b.mode == "port"
	vals := strings.Split(b.fwinfo, ";")
	size := 0
	for _, v := range vals {
		if strings.Contains(v, "width") {
			vv := strings.Split(v, "=")
			sizeval, _ := strconv.ParseInt(vv[1], 10, 32)
			size = int(sizeval) + 1
		}
	}
	return Register{b.id, b.address, masks, noninc, size}
}

func (t *Target) parseregfile(fn string, filebaseaddr uint32) error {
	inp, err := os.Open(fn)
	if err != nil {
		return err
	}
	dec := xml.NewDecoder(inp)
	finished := false
	depth := 0
	tabs := ""
	name := ""
	baseaddr := uint32(0)
	localaddr := uint32(0)
	currentblock := block{}
	currentreg := Register{}
	for !finished {
		tok, err := dec.Token()
		if err == nil {
			if start, ok := tok.(xml.StartElement); ok {
				regtype := ""
				module := ""
				description := ""
				fwinfo := ""
				mode := ""
				mask := uint32(0)
				depth += 1
				tabs += "\t"
				msg := fmt.Sprintf("Start:%s%s, attr = ", tabs, start.Name.Local)
				for _, attr := range start.Attr {
					msg += fmt.Sprintf("%s: %s, ", attr.Name.Local, attr.Value)
					n := attr.Name.Local
					v := attr.Value
					switch {
					case n == "id":
						if v == "TOP" {
							regtype = "TOP"
						} else {
							if name != "" {
								name += "." + v
							} else {
								name = v
							}
						}
					case n == "address":
						addr, _ := strconv.ParseUint(attr.Value, 0, 32)
						localaddr = uint32(addr)
					case n == "mask":
						regtype = "mask"
						maskval, _ := strconv.ParseUint(v, 0, 32)
						mask = uint32(maskval)
					case n == "module":
						regtype = "mod"
						module = v
					case n == "description":
						description = v
					case n == "fwinfo":
						regtype = "blk"
						fwinfo = v
					case n == "mode":
						mode = v
					}
				}
				if regtype == "" {
					regtype = "reg"
				}
				switch {
				case regtype == "blk":
					baseaddr = localaddr + filebaseaddr
					localaddr = uint32(0)
					if currentblock.id != "" {
						t.Regs[currentblock.id] = currentblock.register()
					}
					currentblock = block{name, baseaddr + localaddr, description, fwinfo, mode}
				case regtype == "reg":
					if currentreg.Name != "" {
						t.Regs[currentreg.Name] = currentreg
					}
					masks := make(map[string]uint32)
					noninc := currentblock.mode == "port"
					currentreg = Register{name, baseaddr + localaddr, masks, noninc, 1}
				case regtype == "mask":
					names := strings.Split(name, ".")
					currentreg.Masks[names[len(names)-1]] = mask
				case regtype == "mod":
					modfn := strings.Replace(module, "file://", "", 1)
					dir, _ := filepath.Split(fn)
					modfn = filepath.Join(dir, modfn)
					if err := t.parseregfile(modfn, localaddr); err != nil {
						return err
					}
				}
			} else if _, ok := tok.(xml.EndElement); ok {
				depth -= 1
				tabs = strings.Replace(tabs, "\t", "", 1)
				names := strings.Split(name, ".")
				name = strings.Join(names[:len(names)-1], ".")
			}
		} else {
			finished = true
			if err != io.EOF {
				return err
			}
		}
	}
	t.Regs[currentreg.Name] = currentreg
	t.Regs[currentblock.id] = currentblock.register()
	return error(nil)
}

// Parse an XML file description of the target to automatically produce
// the registers the target contains.
func (t *Target) parse(fn string) error {
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return err
	}
	connections := connectionset{}
	xml.Unmarshal(data, &connections)
	for _, conn := range connections.Conns {
		if conn.Id == t.Name {
			t.dest = strings.Replace(conn.URI, "ipbusudp-2.0://", "", 1)
			//ns := nodes{}
			addr := strings.Replace(conn.Address, "file://", "", 1)
			if err := t.parseregfile(addr, uint32(0)); err != nil {
				return err
			}
		}
	}
	return error(nil)
}
