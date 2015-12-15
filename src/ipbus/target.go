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
    Regs map[string]Register
    // Enable/disable automatic dispatch of transactions.
    // If enabled transactions are sent at the first opportunity when:
    // a) A full UDP packet worth of transactions can be sent
    // b) Target.dt has elapsed since the first queued transaction
    // or
    // c) Target.Dispatch() is called.
    // If disabled transactions are only sent when Target.Dispatch() is called.
    AutoDispatch bool
    dest string
    outgoing, inflight []packet
    nextoutid, nextinid uint32
    requestqueue []usrrequest
}

// Create a new target by parsing an XML file description.
func NewTarget(name, fn string) (Target, error) {
    regs := make(map[string]Register)
    t := Target{Name: name, Regs: regs}
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


// Blocking call to send queued transactions, returns once all replies are received.
func (t Target) Dispatch() {

}

func (t *Target) enqueue(r usrrequest) {
    // Is this a safe way to do things? Different routines might try to do this at once.
    // I might want to do this via a channel instead
    // like: t.requestqueue <- r
    t.requestqueue = append(t.requestqueue, r)
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
