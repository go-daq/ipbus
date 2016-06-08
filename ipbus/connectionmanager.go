package ipbus

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
)

func NewCM(fn string) (CM, error) {
	cm := CM{}
	err := error(nil)
	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return cm, err
	}
	cm.fn = fn
	xml.Unmarshal(data, &cm.connlist)
	return cm, nil
}

type connlist struct {
	Conns []connection `xml:"connection"`
}

// Connection Manager
type CM struct {
	connlist connlist
	fn string
}


func (cm CM) Target(name string) (Target, error) {
	dir := filepath.Dir(cm.fn)
	dest := ""
	addr := ""
	for _, conn := range cm.connlist.Conns {
		if conn.Id == name {
			dest = strings.Replace(conn.URI, "ipbusudp-2.0//", "", 1)
			addr = strings.Replace(conn.Address, "file://", "", 1)
			addr = filepath.Join(dir, addr)
		}
	}
	if dest == "" {
		return Target{}, fmt.Errorf("Connection '%s' not found.", name)
	}
	regs := make(map[string]Register)
	reqs := make(chan usrrequest)
	fp := make(chan bool)
	stop := make(chan bool)
	t := Target{Name: name, Regs: regs, dest: dest, requests: reqs,
				finishpacket: fp, stop: stop}
	t.TimeoutPeriod = DefaultTimeout
	t.AutoDispatch = DefaultAutoDispatch
	err := t.parseregfile(addr, uint32(0))
	return t, err
}
