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
    d := make([]byte, 1500)
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
    Module: mod, inflight:0, maxflight: 4, reporttime: 30 * time.Second}
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

func (i * idlog) secondoldest() (uint16, bool) {
    next := (i.first + 1) % i.max
    return i.ids[next], i.n > 1
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
	nextID, timeoutid uint16 // The packet ID expected next by the hardware.
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
    nverbose int
    bytessent, bytesreceived float64
    reporttime time.Duration
    returnedids []uint16
    returnedindex, returnedsize int
    stopped bool
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
    h.returnedsize = 32
    h.returnedindex = 31
    h.returnedids = make([]uint16, h.returnedsize)
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
        first, ok := h.flyingids.secondoldest()
        if ok {
            dt := h.waittime - time.Since(h.flying[first].reqresp.Sent)
            //fmt.Printf("update timeout = %v, wait time = %d, %v since sent at %v\n", dt, h.waittime, h.flying[first].reqresp.Sent)
            h.timedout = time.NewTicker(dt)
            h.timeoutid = first
        }
    }
}

func (h *HW) handlelost() {
    defer h.clean()
    h.timedout.Stop()
    fmt.Printf("Trying to handle a lost packet with id = %d = 0x%x.\n", h.timeoutid, h.timeoutid)
    fmt.Printf("Previously returned = %v, %d\n", h.returnedids, h.returnedindex)
    status := ipbus.StatusPacket()
    rc := make(chan data.ReqResp)
    h.Send(status, rc)
    rr := <-rc
    statusreply := &ipbus.StatusResp{}
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
        resendpack := ipbus.ResendPacket(h.timeoutid)
        fake := make(chan data.ReqResp)
        h.nverbose = 5
        fmt.Printf("Sending resend request: %v\n", resendpack)
        h.Send(resendpack, fake)
        h.timedout = time.NewTicker(h.waittime)
    } else if !packetreceived {
        fmt.Printf("Packet not received, need to resend original packet (and any following ones).\n")
    } else {
        fmt.Printf("Packet received but not sent, not sure what to do...\n")
    }
    panic(fmt.Errorf("Just panic when a packet is lost."))
}

// Get the device's status to set MTU and next ID.
func (h *HW) ConfigDevice() {
	defer h.clean()
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
            h.timeoutid = first
        }
    }
    return err
}

func (h *HW) sendpack(req request) error {
    //fmt.Printf("Sending packet with ID = %d\n", req.reqresp.Out.ID)
    if h.nverbose > 0 {
        fmt.Printf("Sending request: %v, 0x%x\n", req, req.reqresp.Bytes[:req.reqresp.RespIndex])
    }
    n, err := h.conn.Write(req.reqresp.Bytes[:req.reqresp.RespIndex])
    h.bytessent += float64(n)
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
            h.returnedindex = (h.returnedindex + 1) % h.returnedsize
            h.returnedids[h.returnedindex] = first
        }
    }
}

func (h *HW) clean() {
    h.stopped = true
    if r := recover(); r != nil {
        if err, ok := r.(error); ok {
            fmt.Printf("HW%d caught panic.\n", h.Num)
            ep := data.MakeErrPack(err)
            h.errs <- ep
        }
    }
}

// NB: NEED TO HANDLE STATUS REQUESTS DIFFERENTLY
func (h *HW) Run() {
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
            } else if req.reqresp.Out.Type == ipbus.Resend {
                if err := req.reqresp.EncodeOut(); err != nil {
                    panic(fmt.Errorf("HW%d: %v", h.Num, err))
                }
                req.reqresp.Bytes = req.reqresp.Bytes[:req.reqresp.RespIndex]
                h.sendpack(req)
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
            if h.nverbose > 0 {
                fmt.Printf("Received packet with ID = %d = 0x%x\n", id, id)
                h.nverbose -= 1
            }
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
            fmt.Printf("HW%d: lost a packet :(\nSent ID log: %v\nqueued ID log: %v\nh.nextID = %d", h.Num, h.flyingids, h.queuedids, h.nextID)
            go h.handlelost()
        case <-reportticker.C:
            sentrate := h.bytessent / h.reporttime.Seconds() / 1e6
            recvrate := h.bytesreceived / h.reporttime.Seconds() / 1e6
            fmt.Printf("HW%d sent = %0.2f MB/s, received = %0.2f MB/s\n", h.Num, sentrate, recvrate)
            h.bytessent = 0.0
            h.bytesreceived = 0.0
        }
    }
}

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
    if h.stopped {
        return fmt.Errorf("HW%d is stopped.", h.Num)
    }
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
    defer h.clean()
    running := true
    for running {
        p := emptyPacket()
        n, addr, err := h.conn.ReadFrom(p.Data)
        h.bytesreceived += float64(n)
        if err != nil {
            panic(fmt.Errorf("HW%d read %d bytes from %v: %v\n", h.Num, n, addr, err))
        }
        if h.nverbose > 0 {
            fmt.Printf("Received a packet of %d bytes.\n", n)
        }
        p.Data = p.Data[:n]
        p.RAddr = addr
        h.replies <- p
    }

}
