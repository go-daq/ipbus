// Copyright 2018 The go-daq Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

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
	"fmt"
	"net"
	"time"
)

var nhw = 0

func newhw(conn net.Conn, dt time.Duration) *hw {
	raddr := conn.RemoteAddr()
	hw := hw{Num: nhw, conn: conn, raddr: raddr, waittime: dt,
		nextID: uint16(1), inflight: 0, maxflight: 4,
		reporttime: 30 * time.Second}
	nhw += 1
	//hw.nverbose = 5
	hw.init()
	fmt.Printf("Created new hw: %v\n", hw)
	return &hw
}

type hw struct {
	Num     int
	replies chan hwpacket
	//errs       chan data.ErrPack // Channel to send errors to whomever cares.
	conn       net.Conn // UDP connection with the device.
	raddr      net.Addr // UDP address of the hardware device.
	configured bool     // Flag to ensure connection is configured, etc. before
	// attempting to send data.
	// is assumed to be lost and handled as such.
	statuses          chan targetstatus
	nextID, timeoutid uint16 // The packet ID expected next by the hardware.
	mtu               uint32 // The Maxmimum transmission unit is not currently used,
	// but defines the largest packet size (in bytes) to be
	// sent. It is the hw interface's responsibility to
	// ensure that sent requests and their replies will not
	// overrun this bound. This is not currently implemented.
	// New stuff for multiple packets in flight:
	inflight, maxflight         int
	tosend, flying, replied     *packetlog
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
	h.statuses = make(chan targetstatus, 10)
	h.replies = make(chan hwpacket, 100)
	h.tosend = newpacketlog()
	h.flying = newpacketlog()
	h.replied = newpacketlog()
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
	return fmt.Sprintf("hw%d: LAddr = %v, RAddr = %v, dt = %v", h.Num, h.conn.LocalAddr(), h.raddr, h.waittime)
}

// Connect to hw's UDP socket.
/*
func (h *hw) config() error {
	err := error(nil)
	if h.conn, err = net.DialUDP("udp", nil, h.raddr); err != nil {
		return err
	}
	return error(nil)
}
*/

func (h *hw) updatetimeout() {
	if h.inflight > 0 {
		first, ok := h.flyingids.secondoldest()
		if ok {
			dt := h.waittime
			firstpack, ok := h.flying.get(first)
			if ok {
				dt -= time.Since(firstpack.sent)
			} else {
				fmt.Printf("hw.updatetimeout: Warning: no first pack...\n")
			}
			//fmt.Printf("update timeout = %v, wait time = %d, %v since sent at %v\n", dt, h.waittime, h.flying[first].reqresp.Sent)
			if h.nverbose > 0 {
				fmt.Printf("update timeout = %v, wait time = %d\n", dt, h.waittime)
			}
			if dt < 0 {
				dt = 1000
			}
			h.timedout = time.NewTicker(dt)
			h.timeoutid = first
		} else {
			if h.nverbose > 0 {
				fmt.Printf("updatetimeout() no second oldest...\n")
			}
		}
	} else {
		h.timedout.Stop()
		if h.nverbose > 0 {
			fmt.Printf("updatetimeout() no packets in flight, stopping ticker\n")
		}
	}
}

func (h *hw) handlelost() {
	h.SetVerbose(5)
	h.timedout.Stop()
	h.handlinglost = true
	fmt.Printf("Trying to handle a lost packet with id = %d = 0x%x: %v.\n", h.timeoutid, h.timeoutid, time.Now())
	//fmt.Printf("Previously returned = %v, %d\n", h.returnedids, h.returnedindex)
	//fmt.Printf("sent out = %v\n", h.sentout)
	//fmt.Printf("received = %v\n", h.received)
	//fmt.Printf("returned = %v\n", h.returned)
	fmt.Printf("Flying requests:\n")
	for id, req := range h.flying.getall() {
		fmt.Printf("id = %d = 0x%x: %v\n", id, id, req)
	}
	// Get status
	err := h.sendstatusrequest()
	if err != nil {
		panic(err)
	}
	statusreply := <-h.statuses
	fmt.Printf("Found status: %v\n", statusreply)
	fmt.Printf("Received headers:\n")
	// Check if missing packet was either received or sent
	packetreceived := false
	packetsent := false
	for _, rh := range statusreply.received {
		if rh.pid == h.timeoutid {
			packetreceived = true
			fmt.Printf("    lost packet: %v!\n", rh)
		} else {
			fmt.Printf("    %v\n", rh)
		}
	}
	fmt.Printf("Sent headers:\n")
	for _, sh := range statusreply.sent {
		if sh.pid == h.timeoutid {
			packetsent = true
			fmt.Printf("    lost packet: %v!\n", sh)
		} else {
			fmt.Printf("    %v\n", sh)
		}
	}
	if packetsent {
		err := h.sendresendrequest(h.timeoutid)
		fmt.Printf("Packet sent, need to send resend request.\n")
		if err != nil {
			panic(err)
		}
		h.nverbose = 5
		//h.timedout = time.NewTicker(h.waittime)
		h.updatetimeout()
		fmt.Printf("Handled lost packet that had been sent.\n")
	} else if !packetreceived {
		fmt.Printf("Packet not received, need to resend original packet (and any following ones).\n")
		// Need to resend times out packet and any following packets that are in flight
		// Start with timed out ID, look for it and consecutive IDs in the
		// flying map. If the packet is in the map, just write it
		// to the connection again.
		resendid := h.timeoutid
		flying := true
		resentpack := false
		h.flying.Verbose = true
		for flying {
			var pack *packet
			pack, flying = h.flying.get(resendid)
			if flying {
				// Simply write the data again
				n, err := h.conn.Write(pack.request)
				if err != nil {
					panic(err)
				}
				if n != len(pack.request) {
					panic(fmt.Errorf("hw.handlelost: Sent %d of %d bytes resending packet", n, len(pack.request)))
				}
				pack.sent = time.Now()
				h.flying.add(resendid, pack)
				fmt.Printf("Resent 0x%04x\n", resendid)
				resendid++
				if resendid == 0 {
					resendid = 1
				}
				resentpack = true
			}
		}
		h.flying.Verbose = false
		//h.timedout = time.NewTicker(h.waittime)
		if resentpack {
			h.updatetimeout()
		}
	} else {
		panic(fmt.Errorf("Packet received but not sent, not sure what to do...\n"))
	}
	h.handlinglost = false
	// Better flush the outgoing just in case...
	fmt.Printf("Calling sendnext at end of handlelost...\n")
	h.sendnext()
}

// Get the device's status to set MTU and next ID.
func (h *hw) ConfigDevice() {
	fmt.Printf("hw.ConfigDevice()\n")
	err := h.sendstatusrequest()
	if err != nil {
		panic(err)
	}
	statusreply := <-h.statuses
	h.mtu = statusreply.mtu
	h.nextID = uint16(statusreply.nextid)
	fmt.Printf("Configured device: MTU = %d, next ID = %d\n", h.mtu, h.nextID)
	fmt.Printf("%d response buffers.\n", statusreply.nresponsebuffer)
	h.configured = true
}

// Send the next queued packet if there are slots available
func (h *hw) sendnext() error {
	if h.nverbose > 0 {
		fmt.Printf("hw.sendnext()\n")
	}
	if h.handlinglost {
		fmt.Printf("%s: hw.sendnext(): Handling lost packet, not sending...\n", time.Now())
		return nil
	}
	err := error(nil)
	tosend := h.tosend.getall()
	if h.nverbose > 0 {
		fmt.Printf("h.sendnext: %d in flight, %d max flight, len(tosend) = %d\n", h.inflight, h.maxflight, len(tosend))
	}
	for h.inflight < h.maxflight && len(tosend) > 0 {
		first, ok := h.queuedids.oldest()
		if !ok {
			return fmt.Errorf("Failed to get oldest queued ID")
		}
		h.queuedids.remove()
		pack, _ := h.tosend.get(first)
		h.tosend.remove(first)
		err = h.sendpack(pack)
		if err != nil {
			return err
		}
		pack.sent = time.Now()
		h.flying.add(first, pack)
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
	if h.nverbose > 0 {
		fmt.Printf("Sending request: %v\n", pack)
	}
	n, err := h.conn.Write(pack.request)
	if h.nverbose > 0 {
		fmt.Printf("Status request sent: n, err = %d, %v\n", n, err)
	}
	h.bytessent += float64(n)
	h.packssent += 1.0
	if err != nil {
		return fmt.Errorf("Failed after sending %d bytes: %v", n, err)
	}
	return error(nil)
}

func (h *hw) sendstatusrequest() error {
	data := newStatusPacket()
	fmt.Printf("HW%d sending status request: %x\n", h.Num, data)
	/*
		if h.nverbose > 0 {
			fmt.Printf("HW%d sending status request: %x\n", h.Num, data)
		}
	*/
	n, err := h.conn.Write(data)
	h.bytessent += float64(n)
	h.packssent += 1.0
	if err != nil {
		return fmt.Errorf("hw%d failed after sending %d bytes of status request: %v", h.Num, n, err)
	}
	return error(nil)
}

func (h *hw) sendresendrequest(id uint16) error {
	data := newResendPacket(id)
	n, err := h.conn.Write(data)
	h.bytessent += float64(n)
	h.packssent += 1.0
	h.resent = id
	if err != nil {
		return fmt.Errorf("hw%d failed after sending %d bytes of resend request: %v", h.Num, n, err)
	}
	fmt.Printf("Sent resend request 0x0%04x: %02x\n", id, data)
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
		p, ok := h.replied.get(first)
		if ok {
			h.flyingids.remove()
			h.replied.remove(first)
			h.returned.add(first)
			// Send all the transactions in the packet to their respective channels
			p.send()
			sentrep = true
			h.returnedindex = (h.returnedindex + 1) % h.returnedsize
			h.returnedids[h.returnedindex] = first
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
	/*
		if err := h.config(); err != nil {
			panic(err)
		}
	*/
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
			if h.nverbose > 0 {
				fmt.Printf("%v: hw.Run read from h.incoming: %v\n", time.Now(), pack)
			}
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

			if h.nverbose > 0 {
				fmt.Printf("Adding ID to packet. hw.nextID = %d\n", h.nextID)

			}
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
			h.tosend.add(pack.id, pack)
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
				fmt.Printf("%v: hw.Run received reply with ID = %d = 0x%x\n", time.Now(), id, id)
				h.nverbose -= 1
			}
			if id == 0 { // id == 0 should be status packet
				st, err := parseStatus(rep.Data)
				if err != nil {
					// Handle error
				}
				// Status packets are used either for initial configuration
				// or for deciding what to do with a lost packet
				h.statuses <- st
			} else {
				req, ok := h.flying.get(id)
				if ok {
					h.inflight -= 1
					oldest, _ := h.flyingids.oldest()
					if id == oldest {
						h.timedout.Stop()
						h.updatetimeout()
					}
					h.flying.remove(id)
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
					h.replied.add(id, req)
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
			fmt.Printf("hw%d: lost a packet :(\nSent ID log: %v\nqueued ID log: %v\nh.nextID = %d\n", h.Num, h.flyingids, h.queuedids, h.nextID)
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
	if h.nverbose > 0 {
		fmt.Printf("Packet going into h.incoming: %v\n", p)
	}
	h.incoming <- p
	return error(nil)
}

// Receive incoming packets
func (h *hw) receive() {
	running := true
	for running {
		p := emptyPacket()
		n, err := h.conn.Read(p.Data)
		h.bytesreceived += float64(n)
		h.packsreceived += 1.0
		if err != nil {
			running = false
			fmt.Printf("hw%d not receiving as connection closed: %v\n", h.Num, err)
		} else {
			if h.nverbose > 0 {
				fmt.Printf("%v: hw.receive() packet of %d bytes: 0x%x.\n", time.Now(), n, p.Data[:n])
			}
			p.Data = p.Data[:n]
			p.RAddr = h.raddr
			h.replies <- p
		}
	}
}
