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
    "net"
    "time"
)

func NewHW(raddr net.UDPAddr, dt time.Duration, tosend chan Packet) {
	hw := HW{raddr: raddr, timout: dt, tosend: tosend}
	return hw
}

type HW struct {
	// incoming IPbus packets
	tosend    chan Packet
	replies chan data.Packet
	outp    map[uint16]chan data.Packet
	// map of transaction
	// buffers and map to track which are free?
	buffers [][]byte
	// Channel to send errors to whomever cares.
	errs       chan error
	conn       *net.UDPConn
	raddr      *net.UDPAddr
	configured bool
	timeout    time.Duration
	sent       data.ReqResp
}

func (h *HW) config() error {
    h.conn, err := net.DialUDP("udp", h.raddr)
    return err
}

func (h *HW) Run() {
	if !h.configured {
		if err := h.config(); err != nil {
			panic(err)
		}
	}
	running = true
	for {
		p, ok := <-h.tosend
		if !ok {
			running = false
			break
		}
		// Send packet
		rr := h.send(p)
        tick := time.NewTicker(timeout)
        go h.receive(rr)
        select {
        case reply := <-h.replies:
            // Send reply to the correct channel
            id := reply.Pack.ID
            outps[id] <- reply
            // Remove transactionids from sent

        case _ <- tick:
            // handle timed out request
            fmt.Printf("Transaction %v timed out\n", sent)
		}
		tick.Stop()
		close(outps[id])
		delete(outps, id)
	}
}

/*
   When a user wants to send a packet they are given a channel through
   which to receive replies. The channel is closed when all IPbus
   transactions have received a reply.
*/
func (h *HW) Send(p ipbus.Packet) chan data.ReqResp {
	// Make channel for user to receive replies and add to map
	outp := make(chan data.ReqResp)
	h.outps[p.ID] = outp
	h.tosend <- p
	return outp
}

func (h *HW) send(p ipbus.Packet) data.ReqResp {
	// Select or make a ready buffer
	ibuf := -1
	for i := 0; i < len(h.buffers); i++ {
		if len(h.buffers[i]) == 3 {
			ibuf = i
			break
		}
	}
	selected := *[]byte(nil)
	if ibuf < 0 {
		buffer := make([]byte, 0, 3000)
		h.buffers = append(h.buffers, buffer)
		selected = &(h.buffers[len(h.buffers)-1])
	} else {
		selected = &(h.buffers[ibuf])
	}
	// Make ReqResp
	rr := data.CreateReqResp(p)
	rr.Bytes = selected
	// encode outgoing packet
	if err := rr.Encode(); err != nil {
		panic(err)
	}
	// Send outgoing packet, timestamp ReqResp sent
	n, err := h.conn.Write(rr.Bytes[:rr.RespIndex])
	if n != rr.RespIndex {
		panic(fmt.Errorf("Only sent %d of %d bytes.", n, rr.RespIndex))
	}
    rr.Sent = time.Now()
    return rr
}

func (h HW) receive(rr data.ReqResp) {
	// Write data into buffer from UDP read, timestamp reply and set raddr
    n, addr, err := h.conn.ReadFrom(rr.Bytes[rr.RespIndex:])
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
