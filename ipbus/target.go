package ipbus

import (
	"fmt"
	"net"
    "sort"
	"time"
)

const DefaultTimeout = 10000 * time.Second
const DefaultAutoDispatch = false

type Target struct {
	Name string
	// TimeoutPeriod defines the period to wait after queuing an initial transaction
	// before sending all queued transactions, if AutoDispatch is true.
	// The Timeout is restarted each time a transaction is added when the queue is empty.
	TimeoutPeriod time.Duration
	Regs          map[string]Register
	// Enable/disable automatic dispatch of transactions.
	// If enabled transactions are sent at the first opportunity when:
	// a) A full UDP packet worth of transactions can be sent
	// b) Target.dt has elapsed since the first queued transaction
	// or
	// c) Target.Dispatch() is called.
	// If disabled transactions are only sent when Target.Dispatch() is called.
	AutoDispatch        bool
	dest                string
	outgoing, inflight  []packet
	nextoutid, nextinid uint32
	requests            chan usrrequest
	finishpacket, stop  chan bool
	hw                  *hw
}

// Create a new target by parsing an XML file description.
func New(name, fn string, conn net.Conn) (Target, error) {
		regs := make(map[string]Register)
		reqs := make(chan usrrequest)
		fp := make(chan bool)
		stop := make(chan bool)
		t := Target{Name: name, Regs: regs, requests: reqs, finishpacket: fp, stop: stop}
		t.TimeoutPeriod = DefaultTimeout
		t.AutoDispatch = DefaultAutoDispatch
		t.hw = newhw(conn, t.TimeoutPeriod)
		go t.preparepackets()
		if verbose {
			t.hw.SetVerbose(1)
		}
		go t.hw.Run()
		err := t.parseregfile(fn, "", uint32(0))
	return t, err
}

func (t Target) String() string {
    msg := fmt.Sprintf("Target at %v:\n", t.hw.raddr)
    regnames := []string{}
    for name, _ := range t.Regs {
        regnames = append(regnames, name)
    }
    sort.Strings(regnames)
    for _, regname := range regnames {
        msg += fmt.Sprintf("  %v\n", t.Regs[regname])
    }
    return msg
}

/*
// Enable/disable automatic dispatch of transactions.
// If enabled transactions are sent at the first opportunity when:
// a) A full UDP packet worth of transactions can be sent
// b) Target.dt has elapsed since the first queued transaction
// or
// c) Target.Dispatch() is called.
// If disabled transactions are only sent when Target.Dispatch() is called.
func (t *Target) AllowAutoDispatch(enable bool) {

}
*/

func (t Target) preparepackets() {
	packs := make([]*packet, 0, 8)
	running := true
	for running {
		select {
		case req := <-t.requests:
			if req.dispatch {
				// Dispatch any queued full or partial packets
				if verbose {
					fmt.Printf("Dispatch request. %d packets ready.\n", len(packs))
				}
				for _, p := range packs {
					t.hw.incoming <-p
					//t.send(p)
				}
				packs = []*packet{}
			} else {
				// Add a new request to an existing or new packet
				if len(packs) == 0 {
					packs = append(packs, emptypacket(control))
				}
				p := packs[len(packs)-1]
				// Determine if the current pack has enough space to fit the next request.
				// For read and write requests if the whole transaction it may be split over multiple
				// packets. For RMWbits and RMWsum it just goes into a new packet.
				reqspace, respspace := p.space()
				switch {
				case req.typeid == read || req.typeid == readnoninc:
					nwords := req.nwords
					for nwords > 0 {
						reqspace, respspace := p.space()
						if reqspace < 2 || respspace < 2 {
							packs = append(packs, emptypacket(control))
							p = packs[len(packs)-1]
							reqspace, respspace = p.space()
						}
						ntoread := respspace - 1
						if ntoread > nwords {
							ntoread = nwords
						}
						if ntoread > 255 {
							ntoread = 255
						}
						nwords -= ntoread
						// add read request with ntoread words
						final := nwords == 0
						t := newrequesttransaction(req.typeid, uint8(ntoread), req.addr, req.Input, req.resp, req.byteslice, final)
						if req.typeid == read {
							req.addr += uint32(ntoread)

						}
						p.add(t)
					}
				case req.typeid == write || req.typeid == writenoninc:
					nwords := uint(len(req.Input))
					index := uint(0)
					for nwords > 0 {
                        reqspace, respspace = p.space()
						if reqspace < 3 || respspace < 1 {
							packs = append(packs, emptypacket(control))
							p = packs[len(packs)-1]
							reqspace, respspace = p.space()
						}
						ntowrite := reqspace - 2
						if ntowrite > nwords {
							ntowrite = nwords
						}
						if ntowrite > 255 {
							ntowrite = 255
						}
						nwords -= ntowrite
						final := nwords == 0
						// add write request with ntowrite words
						t := newrequesttransaction(req.typeid, uint8(ntowrite),
							req.addr,
							req.Input[index:index+ntowrite],
							req.resp, req.byteslice, final)
						if err := p.add(t); err != nil {
                            panic(err)
                        }
						if req.typeid == write {
							req.addr += uint32(ntowrite)

						}
						index += ntowrite
					}
				case req.typeid == rmwbits:
					if reqspace < 4 || respspace < 2 {
						packs = append(packs, emptypacket(control))
						p = packs[len(packs)-1]
					}
					// add request
					t := newrequesttransaction(rmwbits, 1, req.addr, req.Input,
						req.resp, req.byteslice, true)
					p.add(t)
				case req.typeid == rmwsum:
					if reqspace < 3 || respspace < 2 {
						packs = append(packs, emptypacket(control))
						p = packs[len(packs)-1]
					}
					// add request
					t := newrequesttransaction(rmwsum, 1, req.addr, req.Input,
						req.resp, req.byteslice, true)
					p.add(t)
				}
			}
		case _, ok := <-t.stop:
			// Stop running when t.stop gets closed
			if !ok {
				running = false
			}
		}
	}
}

// Blocking call to send queued transactions, returns once all replies are received.
func (t Target) Dispatch() {
	// Make sure any partial packets are in the outgoing queue
	r := usrrequest{dispatch: true}
	t.enqueue(r)
	// Send queued packets

}

func (t *Target) send(p *packet) {
	t.hw.Send(p)
}

func (t *Target) enqueue(r usrrequest) {
	t.requests <- r
}

// Read nword words from register reg.
func (t Target) Read(reg Register, nword uint) chan Response {
	resp := make(chan Response)
	tid := read
	if reg.noninc {
		tid = readnoninc
	}
	r := usrrequest{tid, nword, reg.Addr, []uint32{}, resp, false, false}
	t.enqueue(r)
	return resp
}

// Write words in data to register reg.
func (t Target) Write(reg Register, data []uint32) chan Response {
	resp := make(chan Response)
	tid := write
	if reg.noninc {
		tid = writenoninc
	}
	r := usrrequest{tid, uint(len(data)), reg.Addr, data, resp, false, false}
	t.enqueue(r)
	return resp
}

// Update reg by operation: x = (x & andterm) | orterm. Receive previous value of reg in reply.
func (t Target) RMWbits(reg Register, andterm, orterm uint32) chan Response {
	resp := make(chan Response)
	data := []uint32{andterm, orterm}
	r := usrrequest{rmwbits, uint(1), reg.Addr, data, resp, false, false}
	t.enqueue(r)
	return resp
}

// Update reg by operation: x <= (x + addend). Receive previous value of reg in reply.
func (t Target) RMWsum(reg Register, addend uint32) chan Response {
	resp := make(chan Response)
	data := []uint32{addend}
	r := usrrequest{rmwsum, uint(1), reg.Addr, data, resp, false, false}
	t.enqueue(r)
	return resp
}

// Read transaction where reply is kept in []byte array.
func (t Target) ReadB(reg Register, nword uint) chan Response {
	resp := make(chan Response)
	tid := read
	if reg.noninc {
		tid = readnoninc
	}
	r := usrrequest{tid, nword, reg.Addr, []uint32{}, resp, true, false}
	t.enqueue(r)
	return resp
}

// MaskedWrite performs a RMWbits for updarting the masked part of the register to the given value
func (t Target) MaskedWrite(reg Register, mask string, value uint32) (chan Response, error) {
	m, ok := reg.msks[mask]
	if !ok {
		return make(chan Response), fmt.Errorf("MaskedWrite(): reg %v has no mask %s", reg, mask)
	}
	andterm := 0xffffffff ^ m.value
	orterm := value << m.shift
	resp := t.RMWbits(reg, andterm, orterm)
	return resp, nil
}
