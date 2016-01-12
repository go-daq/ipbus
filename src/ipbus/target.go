package ipbus

import (
	"time"
)

const DefaultTimeout = time.Second
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
}

// Create a new target by parsing an XML file description.
func NewTarget(name, fn string) (Target, error) {
	regs := make(map[string]Register)
	reqs := make(chan usrrequest)
	fp := make(chan bool)
	stop := make(chan bool)
	t := Target{Name: name, Regs: regs, requests: reqs, finishpacket: fp, stop: stop}
	t.TimeoutPeriod = DefaultTimeout
	t.AutoDispatch = DefaultAutoDispatch
	err := t.parse(fn)
	return t, err
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
	packs := make([]packet)
	running := true
	for running {
		select {
		case req = <-t.requests:
			if req.dispatch {
				// Dispatch any queued full or partial packets
				for _, p := range packs {
					t.send(&p)
				}
				packs = []packet{}
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
						t := transaction{req.typeid, uint8(ntoread),
							req.addr, req.Input, req.resp,
							req.byteslic, final}
						p.add(t)
					}
				case req.typeid == write || req.typeid == writenoninc:
					nwords := len(req.Input)
					index := 0
					for nwords > 0 {
						if reqspace < 3 || respspace < 1 {
							packs = append(packs, emptypacket(control))
							p = packs[len(packs)-1]
							reqspace, respspace = p.space()
						}
						ntowrite := reqspace - 1
						if ntowrite > nwords {
							ntowrite = nwords
						}
						if ntowrite > 255 {
							ntowrite = 255
						}
						nwords -= ntowrite
						final := nwords == 0
						// add write request with ntowrite words
						t := transaction{req.typeid, uint8(ntowrite),
							req.addr,
							req.Input[index : index+ntowrite],
							req.resp, req.byteslice, final}
						p.add(t)
						index += ntowrite
					}
					for nwords > 0 {
						reqfits = reqspace >= len(req.Input)+1 && respspace >= 2
					}
				case req.typeid == rmwbits:
					if reqspace < 4 || respspace < 2 {
						packs = append(packs, emptypacket(control))
						p = packs[len(packs)-1]
					}
					// add request
					t := transaction{rmwbits, 1, req.addr, req.Input,
						req.resp, req.byteslice, true}
					p.add(t)
				case req.typeid == rmwsum:
					if reqspace < 3 || respspace < 2 {
						packs = append(packs, emptypacket(control))
						p = packs[len(packs)-1]
					}
					// add request
					t := transaction{rmwsum, 1, req.addr, req.Input,
						req.resp, req.byteslice, true}
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
	r := usrrequest{tid, nword, reg.Addr, []uint32{}, resp, false}
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
	r := usrrequest{tid, uint(len(data)), reg.Addr, data, resp, false}
	t.enqueue(r)
	return resp
}

// Update reg by operation: x = (x & andterm) | orterm. Receive previous value of reg in reply.
func (t Target) RMWbits(reg Register, andterm, orterm uint32) chan Response {
	resp := make(chan Response)
	data := []uint32{andterm, orterm}
	r := usrrequest{rmwbits, uint(1), reg.Addr, data, resp, false}
	t.enqueue(r)
	return resp
}

// Update reg by operation: x <= (x + addend). Receive previous value of reg in reply.
func (t Target) RMWsum(reg Register, addend uint32) chan Response {
	resp := make(chan Response)
	data := []uint32{addend}
	r := usrrequest{rmwsum, uint(1), reg.Addr, data, resp, false}
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
	r := usrrequest{tid, nword, reg.Addr, []uint32{}, resp, true}
	t.enqueue(r)
	return resp
}
