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
	num    int
	tosend chan ipbus.Packet // IPbus packets queued up to send to the
	// device. Users should not put packets in the
	// channel directly but should call the
	// hw.Send(packet, chan) method.
	replies chan data.ReqResp   // Responses read from the socket, used by the
	                            // hw.Run() method.
    irecent int
    recent []uint16 // Recent packet IDs
    outps chanmap
	//outps      map[uint16]chan data.ReqResp // map of transaction
	errs       chan error       // Channel to send errors to whomever cares.
	conn       *net.UDPConn     // UDP connection with the device.
	raddr      *net.UDPAddr     // UDP address of the hardware device.
	configured bool     // Flag to ensure connection is configured, etc. before
	// attempting to send data.
	timeout time.Duration // The time period to wait for a reply before it
	// is assumed to be lost and handled as such.
	nextid uint16 // The packet ID expected next by the hardware.
	mtu    uint32 // The Maxmimum transmission unit is not currently used,
	// but defines the largest packet size (in bytes) to be
	// sent. It is the HW interface's responsibility to
	// ensure that sent requests and their replies will not
	// overrun this bound. This is not currently implemented.
}

func (h *HW) init() {
	h.tosend = make(chan ipbus.Packet, 100)
	h.replies = make(chan data.ReqResp)
	//h.outps = make(map[uint16]chan data.ReqResp)
    h.recent = make([]uint16, 100)
    h.outps = newchanmap()
    go h.outps.run()
    h.irecent = 0
	fmt.Printf("initialised %v\n", h)
}

func (h HW) String() string {
	return fmt.Sprintf("HW%d: in = %p, RAddr = %v, dt = %v", h.num, &h.tosend, h.raddr, h.timeout)
}

// Connect to HW's UDP socket.
func (h *HW) config() error {
	err := error(nil)
	if h.conn, err = net.DialUDP("udp", nil, h.raddr); err != nil {
		return err
	}
	return error(nil)
}

// Get the device's status to set MTU and next ID.
func (h *HW) configdevice() {
	status := ipbus.StatusPacket()
	rc := make(chan data.ReqResp)
	h.Send(status, rc)
	rr := <-rc
	statusreply := &ipbus.StatusResp{}
	if err := statusreply.Parse(rr.Bytes[rr.RespIndex:]); err != nil {
		panic(err)
	}
	h.mtu = statusreply.MTU
	h.nextid = uint16(statusreply.Next)
}

/*
 * Continuously send queued packets, handle lost responses and put the
 * responses into user provided channels. Keep running until the h.send
 * channel is closed.
 */
func (h *HW) Run() {
	if !h.configured {
		if err := h.config(); err != nil {
			panic(err)
		}
        go h.configdevice()
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
		rr := h.send(p, false)
		go h.receive(rr)
		received := false
		timedout := false
		lost := &data.ReqResp{}
		for !received {
            tick := time.NewTicker(h.timeout)
			select {
			case reply := <-h.replies:
				forwardreply := true
				if reply.In.Type == ipbus.Status {
                    fmt.Printf("HW%d: Received a Status reply.\n", h.num)
					if timedout {
                        fmt.Printf("HW%d: Expected a status reply because %v was lost.\n", h.num, lost)
						forwardreply = false
						// Check whether the lost packet was received. If it
						// was then request the reply again. Otherwise send
						// the original request again.
						statusreply := ipbus.StatusResp{}
                        if err := statusreply.Parse(reply.Bytes[reply.RespIndex:]); err != nil {
							panic(err)
						}
						if lost.Out.Version != ipbus.Version {
                            panic(fmt.Errorf("HW%d: Trying to handle invalid lost packet: %v", h.num, lost))
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
                            fmt.Printf("HW%d: Lost package was sent, requesting resend: %v.\n", h.num, p)
                            resentreqresp := h.send(p, true)
                            fmt.Printf("HW%d: sent request %v\n", h.num, resentreqresp)
							lost.ClearReply()
							go h.receive(*lost)
						}
						if !reqreceived {
                            fmt.Printf("HW%d: Lost package wasn't received :(, resending at %v.\n", h.num, time.Now())
                            resentreqresp := h.send(lost.Out, true)
                            fmt.Printf("HW%d: resent %v\n", h.num, resentreqresp)
                            go h.receive(*lost)
						}
					} else {
						// If the request was a status then that is fine. If
						// not then I must have sent a status request earlier
						// but received the original reply before the status
						// response, so the status response was received into
						// an unrelated ReqResp.
                        fmt.Printf("HW%d: Wasn't expecting status.\n", h.num)
						if reply.Out.Type != ipbus.Status {
                            fmt.Printf("HW%d: Status in reply to %v\n", h.num, reply.Out)
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
					//fmt.Printf("Sending reply to originator: %d of %v\n", id, h.outps)
                    req := getchan(id)
                    h.outps.read <- req
                    rep := <-req.rep
					if rep.ok {
						rep.c <- reply
						received = true
					} else {
                        fmt.Printf("HW%d WARNING: No channel %d in %v to send reply.\nRecent: %v\n", h.num, id, h.outps, h.recent)
					}
                    h.recent[h.irecent % len(h.recent)] = p.ID
                    h.irecent += 1
                    if timedout {
                        fmt.Printf("HW%d: Handled a lost packet: %v\n", h.num, reply)
                    }
				}
            case now := <-tick.C:
				// handle timed out request
				fmt.Printf("HW%d: Transaction %v timed out %v at %v\n", h.num, rr, h.timeout, now)
				timedout = true
				lost = &rr
				statusreq := ipbus.StatusPacket()
				h.send(statusreq, false)
			}
		tick.Stop()
		}
		if received {
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

/*
   When a user wants to send a packet they also provide a channel
   which will receive the reply.
*/
func (h *HW) Send(p ipbus.Packet, outp chan data.ReqResp) error {
	// Set the packet's ID if it is a control packet with ID != 0
	if p.ID != uint16(0) && p.Type == ipbus.Control {
		p.ID = h.nextid
        if h.nextid == 65535 {
            h.nextid = 1
            fmt.Println("Cycled through package IDs.")
        } else {
            h.nextid += 1
        }
		//fmt.Printf("HW %d: id %d sent, next = %d\n", h.num, p.ID, h.nextid)
	}
    req := addchan(p.ID, outp)
    h.outps.add <- req
    rep := <-req.rep
    if rep.err != nil {
        return rep.err
    }
	h.tosend <- p
	//fmt.Printf("HW%d: %d in tosend channel at %p.\n", h.num, len(h.tosend), &h.tosend)
	return error(nil)
}

func (h *HW) send(p ipbus.Packet, verbose bool) data.ReqResp {
    if p.ID == 1 {
        fmt.Printf("Sending packet with ID = 1: %v\n", p)
    }
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
    if p.Type == ipbus.Resend {
        fmt.Printf("HW%d: Sent resend request at %v\n", h.num, rr.Sent)
    }
    if verbose {
        fmt.Printf("HW%d: sent %v\n", h.num, p)
    }
	return rr
}

func (h HW) receive(rr data.ReqResp) {
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
	if rr.In.ID != rr.Out.ID {
        if rr.In.Type != ipbus.Status {
            panic(fmt.Errorf("Received an unexpected packet ID: %d -> %d", rr.Out.ID, rr.In.ID))
        }
	}
	rr.RAddr = addr
	// Send data.ReqResp containing the buffer into replies
	h.replies <- rr
}

type chanmapitem struct {
    id uint16
    c chan data.ReqResp
    ok bool
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
    m map[uint16] chan data.ReqResp
    add, remove, read chan chanmapreq
}

func newchanmap() chanmap {
    return chanmap{
        m: make(map[uint16] chan data.ReqResp),
        add: make(chan chanmapreq),
        remove: make(chan chanmapreq),
        read: make(chan chanmapreq),
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
            if _, ok := m.m[req.val.id]; ok {
                err = fmt.Errorf("Adding existing channel %d to map %v", req.val.id, m.m)
            }
            m.m[req.val.id] = req.val.c
            req.val.err = err
            req.rep <- req.val
        case req := <-m.remove:
            err := error(nil)
            if _, ok := m.m[req.val.id]; !ok {
                err = fmt.Errorf("Attempt to remove non-existing channel %d to map %v", req.val.id, m.m)
            } else {
                delete(m.m, req.val.id)
            }
            req.val.err = err
            req.rep <- req.val
        case req := <-m.read:
            req.val.c, req.val.ok = m.m[req.val.id]
            req.rep <- req.val
        }
    }
}
