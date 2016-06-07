package ipbus

/*
   hw is the interface to the hardware device. There should only be a single
   hw instance per device. hw should receive all the IPbus packets to send
   to the device. It receives packets from the device and sends then back
   to the originator via a channel supplyed by the originator. The hw instance
   handles lost UDP packets.

   The user supplied packages are handled sequentially. This should simplify
   checking for lost packets. For SoLid operation this should be OK.
   The incoming request is a buffered channel so that users are not blocked
   waiting for packets to be sent. If the incoming packets have a non zero
   transaction ID then the hw instance updates them to be the next ID expected
   by the hardware device.

   hw requires the following IPbus functionality:
       * Encode packet in byte stream to send
       * Parse packet and transaction headers from received byte stream
*/
import (
	"bitbucket.org/NickRyder/goipbus/old/data"
	"fmt"
	"net"
	"bitbucket.org/NickRyder/goipbus/old/glibxml"
	oldipbus "bitbucket.org/NickRyder/goipbus/old/ipbus"
	"time"
)

var nhw = 0

func newhw(num int, mod glibxml.Module, dt time.Duration, errs chan data.ErrPack) *hw {
	raddr, err := net.ResolveUDPAddr("udp", mod.IP)
	if err != nil {
		panic(err)
	}
	hw := hw{Num: num, raddr: raddr, waittime: dt, nextID: uint16(1), errs: errs,
		Module: mod, inflight: 0, maxflight: 4, reporttime: 30 * time.Second}
	hw.init()
	fmt.Printf("Created new hw: %v\n", hw)
	return &hw
}

type hw struct {
	Num        int
	replies    chan hwpacket
	errs       chan data.ErrPack // Channel to send errors to whomever cares.
	conn       *net.UDPConn      // UDP connection with the device.
	raddr      *net.UDPAddr      // UDP address of the hardware device.
	configured bool              // Flag to ensure connection is configured, etc. before
	// attempting to send data.
	// is assumed to be lost and handled as such.
	nextID, timeoutid uint16 // The packet ID expected next by the hardware.
	mtu               uint32 // The Maxmimum transmission unit is not currently used,
	// but defines the largest packet size (in bytes) to be
	// sent. It is the hw interface's responsibility to
	// ensure that sent requests and their replies will not
	// overrun this bound. This is not currently implemented.
	Module glibxml.Module // Addresses of registers, ports, etc.
	// New stuff for multiple packets in flight:
	inflight, maxflight         int
	tosend, flying, replied     map[uint16]*packet
	queuedids, flyingids        idlog
	timedout                    *time.Ticker
	incoming                    chan *packet
	waittime                    time.Duration
	nverbose                    int
	bytessent, bytesreceived    float64
	packssent, packsreceived    float64
	reporttime                  time.Duration
	returnedids                 []uint16
	returnedindex, returnedsize int
	stopped                     bool
	Stop                        chan bool
	sentout, received, returned tracker
	resent                      uint16
	handlinglost                bool
}

func (h *hw) init() {
	h.replies = make(chan hwpacket, 100)
	h.tosend = make(map[uint16]*packet)
	h.flying = make(map[uint16]*packet)
	h.replied = make(map[uint16]*packet)
	h.queuedids = newIDLog(256)
	h.flyingids = newIDLog(32)
	h.timedout = time.NewTicker(10000 * time.Second)
	h.incoming = make(chan *packet, 100)
	h.returnedsize = 32
	h.returnedindex = 31
	h.returnedids = make([]uint16, h.returnedsize)
	h.Stop = make(chan bool)
	h.sentout = newTracker(16)
	h.received = newTracker(16)
	h.returned = newTracker(16)
}

func (h hw) String() string {
	return fmt.Sprintf("hw%d: in = %p, RAddr = %v, dt = %v", h.Num, &h.tosend, h.raddr, h.waittime)
}

// Connect to hw's UDP socket.
func (h *hw) config() error {
	err := error(nil)
	if h.conn, err = net.DialUDP("udp", nil, h.raddr); err != nil {
		return err
	}
	return error(nil)
}

func (h *hw) updatetimeout() {
	if h.inflight > 0 {
		first, ok := h.flyingids.secondoldest()
		if ok {
			dt := h.waittime - time.Since(h.flying[first].sent)
			//fmt.Printf("update timeout = %v, wait time = %d, %v since sent at %v\n", dt, h.waittime, h.flying[first].reqresp.Sent)
			h.timedout = time.NewTicker(dt)
			h.timeoutid = first
		}
	}
}

func (h *hw) handlelost() {
	defer h.clean()
	h.timedout.Stop()
	h.handlinglost = true
	fmt.Printf("Trying to handle a lost packet with id = %d = 0x%x.\n", h.timeoutid, h.timeoutid)
	fmt.Printf("Previously returned = %v, %d\n", h.returnedids, h.returnedindex)
	fmt.Printf("sent out = %v\n", h.sentout)
	fmt.Printf("received = %v\n", h.received)
	fmt.Printf("returned = %v\n", h.returned)
	fmt.Printf("Flying requests:\n")
	for id, req := range h.flying {
		fmt.Printf("id = %d = 0x%x: %v\n", id, id, req)
	}
	//status := oldipbus.StatusPacket()
	rc := make(chan data.ReqResp)
	//h.Send(status, rc)
	rr := <-rc
	statusreply := &oldipbus.StatusResp{}
	if err := statusreply.Parse(rr.Bytes[rr.RespIndex:]); err != nil {
		panic(fmt.Errorf("Failed to parse status packet handling lost: %v", err))
	}
	fmt.Printf("Found status: %v\n", statusreply)
	fmt.Printf("Received headers:\n")
	packetreceived := false
	packetsent := false
	for _, rh := range statusreply.ReceivedHeaders {
		if rh.ID == h.timeoutid {
			packetreceived = true
			fmt.Printf("    lost packet: %v!\n", rh)
		} else {
			fmt.Printf("    %v\n", rh)
		}
	}
	fmt.Printf("Sent headers:\n")
	for _, sh := range statusreply.OutgoingHeaders {
		if sh.ID == h.timeoutid {
			packetsent = true
			fmt.Printf("    lost packet: %v!\n", sh)
		} else {
			fmt.Printf("    %v\n", sh)
		}
	}
	if packetsent {
		fmt.Printf("Packet sent, need to send resend request.\n")
		resendpack := oldipbus.ResendPacket(h.timeoutid)
		//fake := make(chan data.ReqResp)
		h.nverbose = 5
		fmt.Printf("Sending resend request: %v\n", resendpack)
		//h.Send(resendpack, fake)
		h.timedout = time.NewTicker(h.waittime)
	} else if !packetreceived {
		fmt.Printf("Packet not received, need to resend original packet (and any following ones).\n")
	} else {
		fmt.Printf("Packet received but not sent, not sure what to do...\n")
	}
	fmt.Printf("sent out = %v\n", h.sentout)
	fmt.Printf("received = %v\n", h.received)
	fmt.Printf("returned = %v\n", h.returned)
	time.Sleep(500 * time.Millisecond)
	//h.Send(status, rc)
	rr = <-rc
	if err := statusreply.Parse(rr.Bytes[rr.RespIndex:]); err != nil {
		panic(fmt.Errorf("Failed to parse status packet handling lost: %v", err))
	}
	fmt.Printf("Found status: %v\n", statusreply)
	panic(fmt.Errorf("Just panic when a packet is lost."))
}

// Get the device's status to set MTU and next ID.
func (h *hw) ConfigDevice() {
	defer h.clean()
	//status := oldipbus.StatusPacket()
	rc := make(chan data.ReqResp)
	//h.Send(status, rc)
	rr := <-rc
	statusreply := &oldipbus.StatusResp{}
	if err := statusreply.Parse(rr.Bytes[rr.RespIndex:]); err != nil {
		panic(err)
	}
	h.mtu = statusreply.MTU
	h.nextID = uint16(statusreply.Next)
	fmt.Printf("Configured device: MTU = %d, next ID = %d\n", h.mtu, h.nextID)
	h.configured = true
}

// Send the next queued packet if there are slots available
func (h *hw) sendnext() error {
	err := error(nil)
	for h.inflight < h.maxflight && len(h.tosend) > 0 {
		first, ok := h.queuedids.oldest()
		if !ok {
			return fmt.Errorf("Failed to get oldest queued ID")
		}
		h.queuedids.remove()
		pack := h.tosend[first]
		delete(h.tosend, first)
		err = h.sendpack(pack)
		if err != nil {
			return err
		}
		pack.sent = time.Now()
		h.flying[first] = pack
		h.flyingids.add(first)
		h.inflight += 1
		if h.inflight == 1 {
			h.timedout = time.NewTicker(h.waittime)
			h.timeoutid = first
		}
	}
	return err
}

func (h *hw) SetVerbose(n int) {
	h.nverbose = n
}

func (h *hw) sendpack(pack *packet) error {
	//fmt.Printf("Sending packet with ID = %d\n", req.reqresp.Out.ID)
	h.sentout.add(pack.id)
	data := pack.Bytes()
	if h.nverbose > 0 {
		fmt.Printf("Sending request: %v, 0x%x\n", pack, data)
	}
	n, err := h.conn.Write(data)
	h.bytessent += float64(n)
	h.packssent += 1.0
	if err != nil {
		return fmt.Errorf("Failed after sending %d bytes: %v", n, err)
	}
	return error(nil)
}

func (h *hw) returnreply() {
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
			h.returned.add(first)
			// Send all the transactions in the packet to their respective channels
			p.send()
			sentrep = true
			h.returnedindex = (h.returnedindex + 1) % h.returnedsize
			h.returnedids[h.returnedindex] = first
		}
	}
}

func (h *hw) clean() {
	if r := recover(); r != nil {
		if err, ok := r.(error); ok {
			fmt.Printf("hw%d caught panic: %v.\n", h.Num, err)
			h.stopped = true
			h.closeall()
			ep := data.MakeErrPack(err)
			h.errs <- ep
		}
	}
}

func (h *hw) closeall() {
	// This should be updated to close the channels for each transaction that would close its channel after sending.
	/*
		for _, req := range h.replied {
			close(req.dest)
		}
		for _, req := range h.flying {
			close(req.dest)
		}
		for _, req := range h.tosend {
			close(req.dest)
		}
	*/
}

// NB: NEED TO HANDLE STATUS REQUESTS DIFFERENTLY
func (h *hw) Run() {
	defer h.clean()
	if err := h.config(); err != nil {
		panic(err)
	}
	go h.ConfigDevice()
	running := true
	go h.receive()
	reportticker := time.NewTicker(h.reporttime)
	for running {
		select {
		case <-h.Stop:
			h.conn.Close()
			fmt.Printf("hw%d following request to stop.\n", h.Num)
			running = false
		case pack := <-h.incoming:
			// Handle sending out packet
			// Will move status and resend requests to another channel.
			/*
				if req.reqresp.Out.Type == oldipbus.Status {
					if err := req.reqresp.EncodeOut(); err != nil {
						panic(fmt.Errorf("hw%d: %v", h.Num, err))
					}
					req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
					h.sendpack(req)
					h.flying[0] = req
				} else if req.reqresp.Out.Type == oldipbus.Resend {
					if err := req.reqresp.EncodeOut(); err != nil {
						panic(fmt.Errorf("hw%d: %v", h.Num, err))
					}
					req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
					h.resent = req.reqresp.Out.ID
					if h.handlinglost {
						h.sendpack(req)
					} else {
						fmt.Printf("Received a resend request but not hanlding lost packet, treating it like normal.\n")
						h.tosend[req.reqresp.Out.ID] = req
						err := h.queuedids.add(req.reqresp.Out.ID)
						if err != nil {
							panic(err)
						}
						h.sendnext()
					}
				}
			*/
			// To send out status and resend requests implement a port of the above elsewhere

			pack.writeheader(h.nextid())
			//pack.id = h.nextid()
			//req.reqresp.Out.ID = h.nextid()

			// Don't need to encode data, it's already done
			/*
				if err := req.reqresp.EncodeOut(); err != nil {
					panic(fmt.Errorf("hw%d: %v", h.Num, err))
				}
				req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
			*/

			//h.tosend[req.reqresp.Out.ID] = req
			//err := h.queuedids.add(req.reqresp.Out.ID)
			h.tosend[pack.id] = pack
			err := h.queuedids.add(pack.id)
			if err != nil {
				panic(err)
			}
			h.sendnext()
		case rep := <-h.replies:
			// Handle reply
			// Match with requests in flight slots
			// If it's the oldest request send it back, otherwise queue reply
			// If there are queued requests send one
			// Update ticker
			err := rep.header.decode(rep.Data)
			if err != nil {
				// what?
				fmt.Printf("Error decoding packet header: %v\n", err)
			}
			id := rep.header.pid
			h.received.add(id)
			if h.nverbose > 0 {
				fmt.Printf("Received packet with ID = %d = 0x%x\n", id, id)
				h.nverbose -= 1
			}
			if id == 0 {
				// Need to parse reply packet
				/*
					if req, ok := h.flying[id]; ok {
						delete(h.flying, id)
						req.reqresp.Bytes = append(req.reqresp.Bytes, rep.Data...)
						req.reqresp.RAddr = rep.RAddr
						req.reqresp.Received = time.Now()
						if err := req.reqresp.Decode(); err != nil {
							panic(err)
						}
						req.dest <- req.reqresp
					}
				*/
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
					h.sendnext()
					// Need to parse reply packet
					/*
						req.reqresp.Bytes = append(req.reqresp.Bytes, rep.Data...)
						req.reqresp.RAddr = rep.RAddr
						req.reqresp.Received = time.Now()
						if err := req.reqresp.Decode(); err != nil {
							panic(err)
						}
					*/
					err := req.parse(rep.Data)
					if err != nil {
						fmt.Printf("Error parsing reply: %v\n", err)
					}
					h.replied[id] = req
					h.returnreply()
				} else {
					if id == h.resent {
						fmt.Printf("Received a resent packet with ID = %d, but not found ID in h.flying.\n", id)
					} else {
						panic(fmt.Errorf("hw%d: Received packet with ID = %d, no match in %v", h.Num, id, h.inflight))
					}
				}
			}
		case <-h.timedout.C:
			// Handle timeout on oldest packet in flight
			fmt.Printf("hw%d: lost a packet :(\nSent ID log: %v\nqueued ID log: %v\nh.nextID = %d", h.Num, h.flyingids, h.queuedids, h.nextID)
			go h.handlelost()
		case <-reportticker.C:
			dt := h.reporttime.Seconds()
			sentrate := h.bytessent / dt / 1e6
			recvrate := h.bytesreceived / dt / 1e6
			psentrate := h.packssent / dt / 1e3
			precvrate := h.packsreceived / dt / 1e3
			fmt.Printf("hw%d sent = %0.2f kHz, %0.2f MB/s, received = %0.2f kHz, %0.2f MB/s\n", h.Num, psentrate, sentrate, precvrate, recvrate)
			h.bytessent = 0.0
			h.bytesreceived = 0.0
			h.packssent = 0.0
			h.packsreceived = 0.0
		}
	}
}

/*
   When a user wants to send a packet they also provide a channel
   which will receive the reply.
*/

func (h *hw) nextid() uint16 {
	id := h.nextID
	if h.nextID == 65535 {
		h.nextID = 1
	} else {
		h.nextID += 1
	}
	return id
}

func (h *hw) Send(p *packet) error {
	if h.stopped {
		fmt.Printf("Not sending a packet because hw%d is stopped.\n", h.Num)
		return fmt.Errorf("hw%d is stopped.", h.Num)
	}
	h.incoming <- p
	return error(nil)
}

// Send a packet out
func (h *hw) send(data chan *hwpacket, errs chan error) {
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
			errs <- fmt.Errorf("hw%d sent %d byte of data: %v\n", h.Num, n, err)
		}
	}

}

/*
func (h *hw) send(p ipbus.Packet, verbose bool) (data.ReqResp, error) {
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
	//fmt.Printf("hw %d: Sending packet %v to %v: %x\n", h.Num, rr.Out, h.conn.RemoteAddr(), rr.Bytes[:rr.RespIndex])
	n, err := h.conn.Write(rr.Bytes[:rr.RespIndex])
	if err != nil {
		return rr, err
	}
	if n != rr.RespIndex {
		return rr, fmt.Errorf("Only sent %d of %d bytes.", n, rr.RespIndex)
	}
	rr.Sent = time.Now()
	if p.Type == ipbus.Resend {
        fmt.Printf("hw%d: Sent resend request at %v: 0x%x\n", h.Num, rr.Sent, rr.Bytes[:rr.RespIndex])
	}
	if h.nverbose > 0 {
		fmt.Printf("hw%d: sent packet with ID = %d = 0x%x\n", h.Num, rr.Out.ID, rr.Out.ID)
	}
	return rr, error(nil)
}
*/

// Receive incoming packets
func (h *hw) receive() {
	defer h.clean()
	running := true
	for running {
		p := emptyPacket()
		n, addr, err := h.conn.ReadFrom(p.Data)
		h.bytesreceived += float64(n)
		h.packsreceived += 1.0
		if err != nil {
			running = false
			fmt.Printf("hw%d not receiving as connection closed.\n", h.Num)
		} else {
			if h.nverbose > 0 {
				fmt.Printf("Received a packet of %d bytes: 0x%x.\n", n, p.Data[:n])
			}
			p.Data = p.Data[:n]
			p.RAddr = addr
			h.replies <- p
		}
	}
}
