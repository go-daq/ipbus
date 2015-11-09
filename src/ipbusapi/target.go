package ipbusapi

import (
    "time"
)

// Response to a control transaction
type Response struct {
    Err error
    Code InfoCode
    Data []uint32
}

// Response to a control transaction with byte slice data
type ResponseB struct {
    Err error
    Code InfoCode
    Data []byte
}

type Target struct {
    name string
    dt time.Duration
    regnames map[string]Register
    regaddrs map[uint32]Register
}

func NewTarget(fn string) Target {

}

func (t Target) Despatch() {

}

func (t Target) Read(reg Register, nword int) chan Response {

}

func (t Target) ReadNonInc(reg Register, nword int) chan Response {

}

func (t Target) Write(reg Register, data []uint32) chan Response {

}

func (t Target) WriteNonInc(reg Register, data []uint32) chan Response {

}

func (t Target) RMWbits(reg Register, andterm, orterm uint32) chan Response {


}

func (t Target) RMWSum(reg Register, addend uint32) chan Response {

}

func (t Target) ReadB(reg Register, nword int) chan ResponseB {

}

func (t Target) ReadNonIncB(reg Register, nword int) chan ResponseB {

}

