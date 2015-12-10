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

// Read nword words from register reg.
func (t Target) Read(reg Register, nword uint) chan Response {
    return make(chan Response)

}

// Write words in data to register reg.
func (t Target) Write(reg Register, data []uint32) chan Response {
    return make(chan Response)

}

// Update reg by operation: x = (x & andterm) | orterm. Receive previous value of reg in reply.
func (t Target) RMWbits(reg Register, andterm, orterm uint32) chan Response {
    return make(chan Response)

}


// Update reg by operation: x <= (x + addend). Receive previous value of reg in reply.
func (t Target) RMWsum(reg Register, addend uint32) chan Response {
    return make(chan Response)

}

// Read transaction where reply is kept in []byte array.
func (t Target) ReadB(reg Register, nword uint) chan Response {
    return make(chan Response)

}
