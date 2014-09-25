package hw

/*
   HW is the interface to the hardware device. There should only be a single
   HW instance per device. HW should receive all the IPbus packets to send
   to the device. It receives packets from the device and sends then back
   to the originator via a channel. When all transactions receive a reply
   the return channel is closed. The HW instance handles lost UDP packets.

   Each HW instance has a number of byte slice buffers that are each large
   enough to fit the largest UDP packet. Pointers to these buffers are
   passed around. The buffers are flagged as being spare by setting them to
    be three bytes long.

   The user supplied packages are handled sequentially. This should simplify
   checking for lost packets. For SoLid operation this should be OK.
   The incoming request is a buffered channel so that users are not blocked
   waiting for packets to be sent. If the user supplied packages have no
   packet or transaction ID (ie zero value) then HW gives them.

   HW requires the following IPbus functionality:
       * Encode packet in byte stream to send
       * Parse packet and transaction headers from received byte stream
*/

import (
    "data"
    "fmt"
    "ipbus"
    "net"
    "time"
)

var nhw = 0

func NewHW(num int, raddr *net.UDPAddr, dt time.Duration, errs chan error) *HW {
    hw := HW{num: num, raddr: raddr, timeout: dt, nextid: uint16(1), errs: errs}
    hw.init()
    fmt.Printf("Created new HW: %v\n", hw)
	return &hw
}

type HW struct {
    num int
	// incoming IPbus packets
	tosend    chan ipbus.Packet
	replies chan data.ReqResp
	outps map[uint16]chan data.ReqResp
	// map of transaction
	// buffers and map to track which are free?
	buffers [][]byte
	// Channel to send errors to whomever cares.
	errs       chan error
	conn       *net.UDPConn
	raddr      *net.UDPAddr
	configured bool
	timeout    time.Duration
    nextid uint16
}

func (h *HW) init() {
    h.tosend = make(chan ipbus.Packet, 100)
    h.replies = make(chan data.ReqResp)
    h.outps = make(map[uint16]chan data.ReqResp)
    fmt.Printf("initialised %v\n", h)
}

func (h HW) String() string {
    return fmt.Sprintf("HW%d: in = %p, RAddr = %v, dt = %v", h.num, &h.tosend, h.raddr, h.timeout)
}

func (h *HW) config() error {
    err := error(nil)
    h.conn, err = net.DialUDP("udp", nil, h.raddr)
    return err
}

func (h *HW) Run() {
	if !h.configured {
		if err := h.config(); err != nil {
			panic(err)
		}
	}
	running := true
	for running {
        //fmt.Printf("HW %v: expecting info from chan at %p\n", h, &h.tosend)
		p, ok := <-h.tosend
		if !ok {
			running = false
			break
		}
        //fmt.Printf("HW%d: Received packet send, chan map = %v\n", h.num, h.outps)
		// Send packet
		rr := h.send(p)
        tick := time.NewTicker(h.timeout)
        go h.receive(rr)
        timedout := false
        select {
        case reply := <-h.replies:
            // Send reply to the correct channel
            id := reply.Out.ID
            //fmt.Printf("Sending reply to originator: %d of %v\n", id, h.outps)
            c, ok := h.outps[id]
            if ok {
                c <- reply
            } else {
                fmt.Printf("WARNING: No channel %d in %v to send reply.\n", id, h.outps)
            }
        case <- tick.C:
            // handle timed out request
            fmt.Printf("HW %d: Transaction %v timed out %v\n", h.num, rr, h.timeout)
            timedout = true
		}
		tick.Stop()
        if !timedout {
            _, ok := h.outps[rr.Out.ID]
            if ok {
                delete(h.outps, rr.Out.ID)
            } else {
                panic(fmt.Errorf("HW %d: Received transaction %v with no matching channel: %v\n", h.num, rr, h.outps))
            }
        }
	}
}

/*
   When a user wants to send a packet they are given a channel through
   which to receive replies. The channel is closed when all IPbus
   transactions have received a reply.
*/
func (h *HW) Send(p ipbus.Packet, outp chan data.ReqResp) {
	// Make channel for user to receive replies and add to map
    if (p.ID != uint16(0) && p.Type == ipbus.Control) {
        p.ID = h.nextid
        h.nextid += 1
        //fmt.Printf("HW %d: id %d sent, next = %d\n", h.num, p.ID, h.nextid)
    }
	h.outps[p.ID] = outp
    //fmt.Printf("HW %d: Added ID: %v\n", h.num, h.outps)
	h.tosend <- p
    //fmt.Printf("HW%d: %d in tosend channel at %p.\n", h.num, len(h.tosend), &h.tosend)
}

func (h *HW) send(p ipbus.Packet) data.ReqResp {
	// Make ReqResp
	rr := data.CreateReqResp(p)
	// encode outgoing packet
	if err := rr.EncodeOut(); err != nil {
		panic(err)
	}
	// Send outgoing packet, timestamp ReqResp sent
    //fmt.Printf("HW %d: Sending packet %v to %v: %x\n", h.num, rr.Out, h.conn.RemoteAddr(), rr.Bytes[:rr.RespIndex])
	n, err := h.conn.Write(rr.Bytes[:rr.RespIndex])
    if err != nil {
        panic(err)
    }
	if n != rr.RespIndex {
		panic(fmt.Errorf("Only sent %d of %d bytes.", n, rr.RespIndex))
	}
    rr.Sent = time.Now()
    return rr
}

func (h HW) receive(rr data.ReqResp) {
	// Write data into buffer from UDP read, timestamp reply and set raddr
    n, addr, err := h.conn.ReadFrom(rr.Bytes[rr.RespIndex:])
    rr.Bytes = rr.Bytes[:rr.RespIndex + n]
    rr.RespSize = n
    if err != nil {
        panic(err)
    }
    rr.Received = time.Now()
    if err := rr.Decode(); err != nil {
        panic(err)
    }
    if rr.In.ID != rr.Out.ID {
        panic(fmt.Errorf("Received an unexpected packet ID: %d -> %d", rr.Out.ID, rr.In.ID))
    }
    rr.RAddr = addr
	// Send data.ReqResp containing the buffer into replies
    h.replies <- rr
}
