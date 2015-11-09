package ipbusapi

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

type Transaction struct {
    Type TypeID
    NWords uint8
    Addr uint32
    Input []uint32
    byteslice bool
}
