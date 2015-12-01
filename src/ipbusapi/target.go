package ipbusapi

import (
    "time"
)

const defaulttimeout = time.Second
const defaultautodispatch = false


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
    outgoing, inflight []packet
    nextoutid, nextinid uint32
}

// Create a new target by parsing an XML file description.
func NewTarget(name, fn string) (Target, error) {
    names := make(map[string]Register)
    addrs := make(map[uint32]Register)
    t := Target{Name: name, RegName: names, RegAddrs: addrs}
    t.TimeoutPeriod = defaulttimeoutperiod
    t.AutoDispatch = defaultautodispatch
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

// Parse an XML file description of the target to automatically produce
// the registers the target contains.
func (t Target) parse(fn) error {
    return error(nil)
}

// Read nword words from register reg.
func (t Target) Read(reg Register, nword uint) chan Response {

}

// Read nword words from register reg without incrementing address.
func (t Target) ReadNonInc(reg Register, nword uint) chan Response {

}

// Write words in data to register reg.
func (t Target) Write(reg Register, data []uint32) chan Response {

}

// Update reg by operation: x = (x & andterm) | orterm. Receive previous value of reg in reply.
func (t Target) RMWbits(reg Register, andterm, orterm uint32) chan Response {

}


// Update reg by operation: x <= (x + addend). Receive previous value of reg in reply.
func (t Target) RMWsum(reg Register, addend uint32) chan Response {

}

// Read transaction where reply is kept in []byte array.
func (t Target) ReadB(reg Register, nword uint) chan Response {

}
