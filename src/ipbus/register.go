package ipbus

import (
    "fmt"
)

type Register struct {
    Name string
    Addr uint32
    Masks map[string]uint32
    noninc bool
    size int
}

func (r Register) String() string {
    s := fmt.Sprintf("%s at 0x%x", r.Name, r.Addr)
    if r.noninc {
        s += " (non-inc)"
    }
    if len(r.Masks) > 0 {
        s += " ["
    }
    i := 0
    for n, m := range(r.Masks) {
        if i > 0 {
            s += ", "
        }
        s += fmt.Sprintf("%s:0x%x", n, m)
        i += 1
    }
    if i > 0 {
        s += "]"
    }
    return s
}
