package hw

/*
   HW is the interface to the hardware device. There should only be a single
   HW instance per device. HW should receive all the IPbus packets to send
   to the device. It receives packets from the device and sends then back
   to the originator via a channel supplyed by the originator. The HW instance
   handles lost UDP packets.

   The user supplied packages are handled sequentially. This should simplify
   checking for lost packets. For SoLid operation this should be OK.
   The incoming request is a buffered channel so that users are not blocked
   waiting for packets to be sent. If the incoming packets have a non zero
   transaction ID then the HW instance updates them to be the next ID expected
   by the hardware device.

   HW requires the following IPbus functionality:
       * Encode packet in byte stream to send
       * Parse packet and transaction headers from received byte stream
*/
import (
	"crash"
	"data"
	"fmt"
	"glibxml"
	"ipbus"
	"net"
	"time"
)

type packet struct {
    Data []byte
    RAddr net.Addr
}

func emptyPacket() packet {
    d := make([]byte, 2048)
    return packet{Data: d}
}

func newPacket(data []byte) packet {
    return packet{Data: data}
}

type request struct {
    request ipbus.Packet
    reqresp data.ReqResp
    sent, received time.Time
    dest chan data.ReqResp
}

var nhw = 0

func New(num int, mod glibxml.Module, dt time.Duration, exit *crash.Exit, errs chan data.ErrPack) *HW {
	raddr, err := net.ResolveUDPAddr("udp", mod.IP)
	if err != nil {
		panic(err)
	}
	hw := HW{Num: num, raddr: raddr, waittime: dt, nextID: uint16(1), exit: exit, errs: errs,
    Module: mod, inflight:0, maxflight: 1}
	hw.init()
	fmt.Printf("Created new HW: %v\n", hw)
	return &hw
}

type idlog struct {
    ids []uint16
    first, n, max int
}

func (i * idlog) add(id uint16) error {
    if id == 0 {
        return fmt.Errorf("Cannot add id = 0 to id logger.")
    }
    if i.n == i.max {
        return fmt.Errorf("Cannot add id = %d to full id logger.", id)
    }
    next := (i.first + i.n) % i.max
    i.ids[next] = id
    i.n += 1
    return error(nil)
}

func (i * idlog) remove() error {
    if i.n == 0 {
        return fmt.Errorf("Cannot remove id from empty id logger.")
    }
    i.first = (i.first + 1) % i.max
    i.n -= 1
    return error(nil)
}

func (i * idlog) oldest() (uint16, bool) {
    return i.ids[i.first], i.n > 0
}

func (i * idlog) newest() (uint16, bool) {
    newest := (i.first + i.n) % i.max
    return i.ids[newest], i.n > 0
}

func (i * idlog) sorted() []uint16 {
    vals := make([]uint16, 0, i.n)
    for j := 0; j < i.n; j++ {
        next := (i.first + j) % i.max
        vals = append(vals, i.ids[next])
    }
    return vals
}

func (i idlog) String() string {
    return fmt.Sprintf("ids: %v, first = %d, n = %d", i.ids, i.first, i.n)
}


func newIDLog(size int) idlog {
    ids := make([]uint16, size)
    return idlog{ids: ids, first: 0, n: 0, max: size}
}

type HW struct {
	Num    int
	replies chan packet
	exit       *crash.Exit
	errs       chan data.ErrPack // Channel to send errors to whomever cares.
	conn       *net.UDPConn      // UDP connection with the device.
	raddr      *net.UDPAddr      // UDP address of the hardware device.
	configured bool              // Flag to ensure connection is configured, etc. before
	// attempting to send data.
	// is assumed to be lost and handled as such.
	nextID uint16 // The packet ID expected next by the hardware.
	mtu    uint32 // The Maxmimum transmission unit is not currently used,
	// but defines the largest packet size (in bytes) to be
	// sent. It is the HW interface's responsibility to
	// ensure that sent requests and their replies will not
	// overrun this bound. This is not currently implemented.
	Module glibxml.Module // Addresses of registers, ports, etc.
    // New stuff for multiple packets in flight:
    inflight, maxflight int
    tosend, flying, replied map[uint16]request
    queuedids, flyingids idlog
    timedout * time.Ticker
    incoming chan request
    waittime time.Duration
}

func (h *HW) init() {
	h.replies = make(chan packet, 100)
    h.tosend = make(map[uint16]request)
    h.flying = make(map[uint16]request)
    h.replied= make(map[uint16]request)
    h.queuedids = newIDLog(256)
    h.flyingids = newIDLog(32)
    h.timedout = time.NewTicker(10000 * time.Second)
    h.incoming = make(chan request, 100)
}

func (h HW) String() string {
	return fmt.Sprintf("HW%d: in = %p, RAddr = %v, dt = %v", h.Num, &h.tosend, h.raddr, h.waittime)
}

// Connect to HW's UDP socket.
func (h *HW) config() error {
	err := error(nil)
	if h.conn, err = net.DialUDP("udp", nil, h.raddr); err != nil {
		return err
	}
	return error(nil)
}

func (h *HW) updatetimeout() {
    if h.inflight > 0 {
        first, ok := h.flyingids.oldest()
        if ok {
            dt := h.waittime - time.Since(h.flying[first].sent)
            h.timedout = time.NewTicker(dt)
        }
    }
}

// Get the device's status to set MTU and next ID.
func (h *HW) ConfigDevice() {
	defer h.exit.CleanExit("HW.ConfigDevice()")
	status := ipbus.StatusPacket()
	rc := make(chan data.ReqResp)
	h.Send(status, rc)
	rr := <-rc
	statusreply := &ipbus.StatusResp{}
	if err := statusreply.Parse(rr.Bytes[rr.RespIndex:]); err != nil {
		panic(err)
	}
	h.mtu = statusreply.MTU
	h.nextID = uint16(statusreply.Next)
	fmt.Printf("Configured device: MTU = %d, next ID = %d\n", h.mtu, h.nextID)
    h.configured = true
}

// Send the next queued packet if there are slots available
func (h * HW) sendnext() error {
    err := error(nil)
    for h.inflight < h.maxflight && len(h.tosend) > 0 {
        first, ok := h.queuedids.oldest()
        if !ok {
            return fmt.Errorf("Failed to get oldest queued ID")
        }
        h.queuedids.remove()
        req := h.tosend[first]
        delete(h.tosend, first)
        err = h.sendpack(req)
        if err != nil {
            return err
        }
        req.reqresp.Sent = time.Now()
        h.flying[first] = req
        h.flyingids.add(first)
        h.inflight += 1
        if h.inflight == 1 {
            h.timedout = time.NewTicker(h.waittime)
        }
    }
    return err
}

func (h *HW) sendpack(req request) error {
    n, err := h.conn.Write(req.reqresp.Bytes[:req.reqresp.RespIndex])
    if err != nil {
        return fmt.Errorf("Failed after sending %d bytes: %v", n, err)
    }
    return error(nil)
}

func (h * HW) returnreply() {
    sentrep := true
    for sentrep {
        sentrep = false
        first, ok := h.flyingids.oldest()
        if !ok {
            break
        }
        p, ok := h.replied[first]
        if ok {
            h.flyingids.remove()
            delete(h.replied, first)
            p.dest <- p.reqresp
            sentrep = true
        }
    }
}

// NB: NEED TO HANDLE STATUS REQUESTS DIFFERENTLY
func (h *HW) Run() {
    if err := h.config(); err != nil {
        panic(err)
    }
    go h.ConfigDevice()
    running := true
    go h.receive()
    for running {
        select {
        case req := <-h.incoming:
            // Handle incoming request
            // If there are flight slots free send and update ticker, if not queue
            if req.reqresp.Out.Type == ipbus.Status {
                if err := req.reqresp.EncodeOut(); err != nil {
                    panic(fmt.Errorf("HW%d: %v", h.Num, err))
                }
                req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
                h.sendpack(req)
                h.flying[0] = req
            } else {
                req.reqresp.Out.ID = h.nextid()
                if err := req.reqresp.EncodeOut(); err != nil {
                    panic(fmt.Errorf("HW%d: %v", h.Num, err))
                }
                req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
                h.tosend[req.reqresp.Out.ID] = req
                err := h.queuedids.add(req.reqresp.Out.ID)
                if err != nil {
                    panic(err)
                }
                h.sendnext()
            }
        case rep := <-h.replies:
            // Handle reply
            // Match with requests in flight slots
            // If it's the oldest request send it back, otherwise queue reply
            // If there are queued requests send one
            // Update ticker
            id := uint16(rep.Data[1]) << 8
            id |= uint16(rep.Data[2])
            if id == 0 {
                if req, ok := h.flying[id]; ok {
                    delete(h.flying, id)
                    req.reqresp.Bytes = append(req.reqresp.Bytes, rep.Data...)
                    req.reqresp.RAddr = rep.RAddr
                    req.received = time.Now()
                    if err := req.reqresp.Decode(); err != nil {
                        panic(err)
                    }
                    req.dest <- req.reqresp
                }
            } else {
                req, ok := h.flying[id]
                if ok {
                    h.inflight -= 1
                    oldest, _ := h.flyingids.oldest()
                    if id == oldest {
                        h.timedout.Stop()
                        h.updatetimeout()
                    }
                    delete(h.flying, id)
                    req.reqresp.Bytes = append(req.reqresp.Bytes, rep.Data...)
                    req.reqresp.RAddr = rep.RAddr
                    req.reqresp.Received = time.Now()
                    if err := req.reqresp.Decode(); err != nil {
                        panic(err)
                    }
                    h.replied[id] = req
                    h.sendnext()
                    h.returnreply()
                } else {
                    panic(fmt.Errorf("HW%d: Received packet with ID = %d, no match in %v", h.Num, id, h.inflight))
                }
            }
        case <-h.timedout.C:
            // Handle timeout on oldest packet in flight
            panic(fmt.Errorf("HW%d: lost a packet :(", h.Num))
        }
    }
}
/*
 * Continuously send queued packets, handle lost responses and put the
 * responses into user provided channels. Keep running until the h.send
 * channel is closed.
 */
/*
func (h *HW) Run() {
	defer h.exit.CleanExit("HW.Run()")
	if !h.configured {
		if err := h.config(); err != nil {
			panic(err)
		}
		go h.ConfigDevice()
	}
	running := true
    lastreceived := uint16(0)
	ntimeout := 0
	for running {
		//fmt.Printf("HW %v: expecting info from chan at %p\n", h, &h.tosend)
		p, ok := <-h.tosend
		if !ok {
			running = false
			break
		}
		//fmt.Printf("HW%d: Received packet send, chan map = %v\n", h.Num, h.outps)
		// Send packet
		rr, err := h.send(p, false)
		if err != nil {
			panic(err)
		}
		go h.receive(rr)
		received := false
		timedout := false
		lost := &data.ReqResp{}
		for running && !received {
			tick := time.NewTicker(h.timeout)
			select {
			case reply := <-h.replies:
				forwardreply := true
				if reply.In.Type == ipbus.Status {
					fmt.Printf("HW%d: Received a Status reply.\n", h.Num)
					if timedout {
						fmt.Printf("HW%d: Expected a status reply because %v was lost.\n", h.Num, lost)
                        fmt.Printf("HW%d: last received ID = %d = 0x%08x.\n", h.Num, lastreceived, lastreceived)
						forwardreply = false
						// Check whether the lost packet was received. If it
						// was then request the reply again. Otherwise send
						// the original request again.
						statusreply := ipbus.StatusResp{}
						if err := statusreply.Parse(reply.Bytes[reply.RespIndex:]); err != nil {
							panic(err)
						}
                        h.nextid = uint16(statusreply.Next)
						if lost.Out.Version != ipbus.Version {
							panic(fmt.Errorf("HW%d: Trying to handle invalid lost packet: %v", h.Num, lost))
						}
						reqreceived := false
						replysent := false
						for _, head := range statusreply.ReceivedHeaders {
							if head.ID == lost.Out.ID {
								reqreceived = true
							}
						}
						for _, head := range statusreply.OutgoingHeaders {
							if head.ID == lost.Out.ID {
								replysent = true
							}
						}
						if replysent {
							p := ipbus.ResendPacket(lost.Out.ID)
							fmt.Printf("HW%d: Lost package was sent, requesting resend: %v.\n", h.Num, p)
							resentreqresp, err := h.send(p, true)
							if err != nil {
								panic(err)
							}
							fmt.Printf("HW%d: sent request %v\n", h.Num, resentreqresp)
							lost.ClearReply()
							go h.receive(*lost)
						}
						if !reqreceived {
							fmt.Printf("HW%d: Lost package wasn't received :(, resending at %v.\n", h.Num, time.Now())
                            fmt.Printf("Setting packet ID = %d = 0x%x\n", h.nextid, h.nextid)
                            oldid := lost.Out.ID
                            newid := h.nextid
                            fmt.Printf("Cleaning up reply channel map.\nGetting reply channel for old ID = %d\n", oldid)
                            req := getchan(oldid)
                            h.outps.read <- req
                            rep := <-req.rep
                            fmt.Printf("resp = %v\n", rep)
                            if rep.ok {
                                ch := rep.c
                                fmt.Printf("Got channel for old ID, removing channel with ID = %d\n", oldid)
                                req = remchan(oldid)
                                h.outps.remove <- req
                                rep = <-req.rep
                                if rep.err != nil {
                                    panic(err)
                                }
                                fmt.Printf("Removing channel with new ID = %d\n", newid)
                                req = remchan(newid)
                                h.outps.remove <- req
                                rep = <-req.rep
                                if rep.err != nil {
                                    fmt.Printf("Err = %v\n", rep.err)
                                }
                                fmt.Printf("resp = %v\n", rep)
                                fmt.Printf("Removed old channel, adding channel with new ID = %d\n", newid)
                                req = addchan(newid, ch)
                                h.outps.add <- req
                                rep = <-req.rep
                                if rep.err != nil {
                                    panic(rep.err)
                                }
                                fmt.Printf("resp = %v\n", rep)
                            } else {
                                panic(fmt.Errorf("Failed updating output channel %d -> %d", oldid, newid))
                            }
                            fmt.Printf("Finished cleaning up chanmap.\n")
                            lost.Out.ID = newid
                            h.nextid += 1
                            fmt.Printf("Lost ID = %d = 0x%x\n", lost.Out.ID, lost.Out.ID)
							resentreqresp, err := h.send(lost.Out, true)
							if err != nil {
								panic(err)
							}
							fmt.Printf("HW%d: resent %v\n", h.Num, resentreqresp)
							go h.receive(*lost)
						}
					} else {
						// If the request was a status then that is fine. If
						// not then I must have sent a status request earlier
						// but received the original reply before the status
						// response, so the status response was received into
						// an unrelated ReqResp.
						fmt.Printf("HW%d: Wasn't expecting status.\n", h.Num)
						if reply.Out.Type != ipbus.Status {
							fmt.Printf("HW%d: Status in reply to %v\n", h.Num, reply.Out)
							forwardreply = false
							// Since the status request is unrelated clear the
							// reply bytes and tell it to receive again.
							reply.ClearReply()
							go h.receive(reply)
						}

					}

				}
				if forwardreply {
					// Send reply to the correct channel
					id := reply.Out.ID
                    lastreceived = id
					//fmt.Printf("Sending reply to originator: %d of %v\n", id, h.outps)
					req := getchan(id)
					h.outps.read <- req
					rep := <-req.rep
					if rep.ok {
						rep.c <- reply
						received = true
					} else {
						fmt.Printf("HW%d WARNING: No channel %d in %v to send reply.\nRecent: %v\n", h.Num, id, h.outps, h.recent)
					}
					h.recent[h.irecent%len(h.recent)] = p.ID
					h.irecent += 1
					if timedout {
						fmt.Printf("HW%d: Handled a lost packet: %v\n", h.Num, reply)
						ntimeout = 0
					}
				}
			case now := <-tick.C:
				// handle timed out request
                h.nverbose = 5
				ntimeout += 1
				if ntimeout > 10 {
					running = false
					panic(fmt.Errorf("HW%d: %d lost packets in a row.", h.Num, ntimeout))
				}
				fmt.Printf("HW%d: Transaction %v timed out %v at %v\n", h.Num, rr, h.timeout, now)
				timedout = true
				lost = &rr
				statusreq := ipbus.StatusPacket()
				h.send(statusreq, false)
			case errp := <-h.errs:
				running = false
				fmt.Printf("HW.Run() noticed a panic, stopping.\n")
				h.errs <- errp
			}
			tick.Stop()
		}
		if received {
            if h.nverbose > 0 {
                fmt.Printf("HW%d: removing chanmap entry for packet ID = %d\n", rr.Out.ID)
            }
			// Remove the channel from the output map
			req := remchan(rr.Out.ID)
			h.outps.remove <- req
			rep := <-req.rep
			if rep.err != nil {
				panic(rep.err)
			}
		}
	}
}
*/

/*
   When a user wants to send a packet they also provide a channel
   which will receive the reply.
*/

func (h *HW) nextid() uint16 {
    id := h.nextID
    if h.nextID == 65535 {
        h.nextID = 1
    } else {
        h.nextID += 1
    }
    return id
}

func (h *HW) Send(p ipbus.Packet, outp chan data.ReqResp) error {
    rr := data.CreateReqResp(p)
    req := request{request: p, reqresp: rr, dest: outp}
    h.incoming <- req
    return error(nil)
}

// Send a packet out
func (h *HW) send(data chan *packet, errs chan error) {
    running := true
    for running {
        p, ok := <-data
        if !ok {
            running = false
            continue
        }
        fmt.Printf("Sent a packet\n")
        n, err := h.conn.Write(p.Data)
        if err != nil {
            errs <- fmt.Errorf("HW%d sent %d byte of data: %v\n", h.Num, n, err)
        }
    }

}

/*
func (h *HW) send(p ipbus.Packet, verbose bool) (data.ReqResp, error) {
//	   if p.ID == 1 {
//	       fmt.Printf("Sending packet with ID = 1: %v\n", p)
//	   }
	// Make ReqResp
	rr := data.CreateReqResp(p)
	// encode outgoing packet
	if err := rr.EncodeOut(); err != nil {
		return rr, err
	}
	// Send outgoing packet, timestamp ReqResp sent
	//fmt.Printf("HW %d: Sending packet %v to %v: %x\n", h.Num, rr.Out, h.conn.RemoteAddr(), rr.Bytes[:rr.RespIndex])
	n, err := h.conn.Write(rr.Bytes[:rr.RespIndex])
	if err != nil {
		return rr, err
	}
	if n != rr.RespIndex {
		return rr, fmt.Errorf("Only sent %d of %d bytes.", n, rr.RespIndex)
	}
	rr.Sent = time.Now()
	if p.Type == ipbus.Resend {
        fmt.Printf("HW%d: Sent resend request at %v: 0x%x\n", h.Num, rr.Sent, rr.Bytes[:rr.RespIndex])
	}
	if h.nverbose > 0 {
		fmt.Printf("HW%d: sent packet with ID = %d = 0x%x\n", h.Num, rr.Out.ID, rr.Out.ID)
	}
	return rr, error(nil)
}
*/

// Receive incoming packets
func (h *HW) receive() {
    running := true
    for running {
        p := emptyPacket()
        n, addr, err := h.conn.ReadFrom(p.Data)
        if err != nil {
            panic(fmt.Errorf("HW%d read %d bytes from %v: %v\n", h.Num, n, addr, err))
        }
        p.Data = p.Data[:n]
        p.RAddr = addr
        h.replies <- p
    }

}
/*
func (h * HW) receive(rr data.ReqResp) {
    msg := fmt.Sprintf("HW.receive() on HW%d", h.Num)
	defer h.exit.CleanExit(msg)
	// Write data into buffer from UDP read, timestamp reply and set raddr
	n, addr, err := h.conn.ReadFrom(rr.Bytes[rr.RespIndex:])
	rr.Bytes = rr.Bytes[:rr.RespIndex+n]
	rr.RespSize = n
	if err != nil {
		panic(err)
	}
	rr.Received = time.Now()
	if err := rr.Decode(); err != nil {
		panic(err)
	}
    if h.nverbose > 0 {
        fmt.Printf("HW%d: Received packet with ID = %d = 0x%x\n", h.Num, rr.In.ID, rr.In.ID)
        h.nverbose -= 1
    }
	if rr.In.ID != rr.Out.ID {
		if rr.In.Type != ipbus.Status {
			panic(fmt.Errorf("Received an unexpected packet ID: %d -> %d", rr.Out.ID, rr.In.ID))
		}
	}
	rr.RAddr = addr
	// Send data.ReqResp containing the buffer into replies
	h.replies <- rr
}
*/

/*
type chanmapitem struct {
	id  uint16
	c   chan data.ReqResp
	ok  bool
	err error
}

type chanmapreq struct {
	val chanmapitem
	rep chan chanmapitem
}

func getchan(id uint16) chanmapreq {
	val := chanmapitem{id: id}
	rep := make(chan chanmapitem)
	return chanmapreq{val, rep}
}

func remchan(id uint16) chanmapreq {
	return getchan(id)
}

func addchan(id uint16, c chan data.ReqResp) chanmapreq {
	val := chanmapitem{id: id, c: c}
	rep := make(chan chanmapitem)
	return chanmapreq{val, rep}
}


type chanmap struct {
	m                 []chan data.ReqResp
	exists            []bool
	add, remove, read chan chanmapreq
}

func (cm *chanmap) movechan(oldid, newid uint16) {
    cm.exists[oldid] = false
    ch := cm.m[oldid]
    cm.m[newid] = ch
    cm.exists[newid] = true
}

func newchanmap() chanmap {
	return chanmap{
		m:      make([]chan data.ReqResp, 65536),
		exists: make([]bool, 65536),
		add:    make(chan chanmapreq),
		remove: make(chan chanmapreq),
		read:   make(chan chanmapreq),
	}
}

func (m *chanmap) run() {
	running := true
	for running {
		select {
		case req, open := <-m.add:
			if !open {
				running = false
				break
			}
			err := error(nil)
			if m.exists[req.val.id] {
				err = fmt.Errorf("Adding existing channel %d to map %v", req.val.id, m.m)
			}
			m.m[req.val.id] = req.val.c
			m.exists[req.val.id] = true
//			   if _, ok := m.m[req.val.id]; ok {
//			       err = fmt.Errorf("Adding existing channel %d to map %v", req.val.id, m.m)
//			   }
//			   m.m[req.val.id] = req.val.c
			req.val.err = err
			req.rep <- req.val
		case req := <-m.remove:
			err := error(nil)
			if !m.exists[req.val.id] {
				err = fmt.Errorf("Attempt to remove non-existing channel %d to map %v", req.val.id, m.m)
			}
			m.exists[req.val.id] = false
//			   if _, ok := m.m[req.val.id]; !ok {
//			       err = fmt.Errorf("Attempt to remove non-existing channel %d to map %v", req.val.id, m.m)
//			   } else {
//			       delete(m.m, req.val.id)
//			   }
			req.val.err = err
			req.rep <- req.val
		case req := <-m.read:
			req.val.ok = m.exists[req.val.id]
			req.val.c = m.m[req.val.id]
//			   req.val.c, req.val.ok = m.m[req.val.id]
			req.rep <- req.val
		}
	}
}
*/
