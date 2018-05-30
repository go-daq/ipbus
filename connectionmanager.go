package ipbus

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net"
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
	for _, conn := range cm.connlist.Conns {
		cm.Devices = append(cm.Devices, conn.Id)
	}
	return cm, nil
}

type connlist struct {
	Conns []connection `xml:"connection"`
}

type connection struct {
	Id      string `xml:"id,attr"`
	URI     string `xml:"uri,attr"`
	Address string `xml:"address_table,attr"`
}

// Connection Manager
type CM struct {
	Devices  []string
	connlist connlist
	fn       string
}

func (cm CM) Target(name string) (Target, error) {
	dir := filepath.Dir(cm.fn)
	dest := ""
	addr := ""
	for _, conn := range cm.connlist.Conns {
		if conn.Id == name {
			dest = strings.Replace(conn.URI, "ipbusudp-2.0://", "", 1)
			addr = strings.Replace(conn.Address, "file://", "", 1)
			addr = filepath.Join(dir, addr)
		}
	}
	if dest == "" {
		return Target{}, fmt.Errorf("Connection '%s' not found.", name)
	}
	raddr, err := net.ResolveUDPAddr("udp4", dest)
	if err != nil {
		return Target{}, err
	}
	conn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		panic(err)
	}
	t, err := New("dummy", addr, conn)
	return t, err
}
