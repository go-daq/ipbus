package data

import (
    "fmt"
    "old/ipbus"
    "net"
    "os/exec"
    "runtime"
    "strconv"
    "strings"
    "time"
)

func convert(hash string) ([]byte, error) {
    val := make([]byte, 0, len(hash) / 2)
    if len(hash) % 2 != 0 {
        return val, fmt.Errorf("Odd number of chars, not sha1 hash.")
    }
    for i := 0; i < len(hash) / 2; i++ {
        n := 2 * i
        s := hash[n:n + 2]
        b, err := strconv.ParseUint(s, 16, 8)
        if err != nil {
            return val, err
        }
        if b > 255 {
            return val, fmt.Errorf("Invalid %dth byte: %d in hash %s", i, b, hash)
        }
        val = append(val, uint8(b))
    }
    return val, nil
}

var NotCommittedError = fmt.Errorf("Code not committed.")

type Commit struct {
    Hash []byte
    Modified bool
}

func (c Commit) String() string {
    s := fmt.Sprintf("%x", c.Hash)
    if c.Modified {
        s += " Modified"
    }
    return s
}

func getcommit() (Commit, error) {
    c := Commit{}
    cmd := exec.Command("git", "log", "-n", "1")
    out, err := cmd.Output()
    if err != nil {
        return c, err
    }
    //fmt.Printf("%s\n", out)
    invalidlog := fmt.Errorf("Invalid git log: %s", out)
    commitlines := strings.Split(string(out), "\n")
    if len(commitlines) < 1 {
        return c, invalidlog
    }
    commitline := strings.Split(commitlines[0], " ")
    if commitline[0] != "commit" {
        return c, invalidlog
    }
    hash, err := convert(commitline[1])
    if err != nil {
        return c, invalidlog
    }
    c.Hash = hash
    cmd = exec.Command("git", "diff")
    out, err = cmd.Output()
    if err != nil {
        return c, err
    }
    //fmt.Printf("%s\n", out)
    if len(out) > 0 {
        c.Modified = true
    }
    return c, error(nil)
}

type Run struct{
    Num uint32
    Name string
    Start, End time.Time
    Duration time.Duration
    Commit Commit
    Threshold, MuThreshold int
    Coincidence bool
    Rate float64
}

func NewRun(n uint32, name string, dt time.Duration, threshold, muthreshold int, randomrate float64, coincidence bool) (Run, error) {
    now := time.Now()
    r := Run{Num: n, Name: name, Start: now, End: now.Add(dt), Duration: dt,
             Threshold: threshold, MuThreshold: muthreshold,
             Rate: randomrate, Coincidence: coincidence}
    c, err := getcommit()
    if err != nil {
        return r, err
    }
    r.Commit = c
    return r, error(nil)
}

type Config struct{
    Vs, Ts []float64
    Last time.Time
}

type ReqResp struct{
    Out ipbus.Packet
    In ipbus.PackHeader
    Bytes []byte
    Sent, Received time.Time
    RAddr net.Addr
    RespIndex, RespSize int
}

func (r ReqResp) String() string {
    return fmt.Sprintf("out = %v, in = %v, %x, %v, %v, %v, %d, %d", r.Out, r.In, r.Bytes[r.RespIndex + r.RespSize], r.Sent, r.Received, r.RAddr, r.RespIndex, r.RespSize)
}

func (r *ReqResp) ClearReply() {
    for i := r.RespIndex; i < len(r.Bytes); i++ {
        r.Bytes[i] = 0x0
    }
}

var ReqRespEncodeVersion = uint16(0x0000)

func (r ReqResp) Encode() ([]byte, error) {
    out := make([]byte, 0, len(r.Bytes) + 32)
    /* Write my header: 
        8 minor bits of IP, 0 bits 0x0, 16 bits length
        time sent - 64 bit
        latency [microsecond] - 32 bit
    */
    host, _, err := net.SplitHostPort(r.RAddr.String())
    if err != nil {
        return []byte{}, err
    }
    ip := net.ParseIP(host)
    ipv4 := []byte(ip[12:])
    //out = append(out, ipv4...)
    out = append(out, uint8(ipv4[3]))
    out = append(out, uint8(0))
    words := uint16(len(r.Bytes) / 4) + 4
    out = append(out, uint8((words & 0xff00) >> 8))
    out = append(out, uint8((words & 0x00ff)))
    sentnano := r.Sent.UnixNano()
    for i := 0; i < 8; i++ {
        shift := uint((7 - i) * 8)
        mask := int64(0xff << shift)
        out = append(out, uint8((sentnano & mask) >> shift))
    }
    latency := r.Received.Sub(r.Sent).Seconds()
    latencyus := uint32(latency * 1e6)
    for i := 0; i < 4; i++ {
        shift := uint((3 - i) * 8)
        mask := uint32(0xff << shift)
        out = append(out, uint8((latencyus & mask) >> shift))
    }
    out = append(out, r.Bytes...)
    return out, error(nil)
}

func (r *ReqResp) EncodeOut() error {
    r.Bytes = r.Bytes[:0]
    enc, err := r.Out.Encode()
    if err != nil {
        return err
    }
    r.Bytes = append(r.Bytes, enc...)
    r.RespIndex = len(r.Bytes)
    for i := 0; i < 1500; i++ {
        r.Bytes = append(r.Bytes, 0x0)
    }
    return error(nil)
}

func (r *ReqResp) Decode() error {
    //fmt.Printf("Decoding from loc = %d, %d bytes\n", r.RespIndex, len(r.Bytes))
    //fmt.Println("Decode done.")
    r.In = ipbus.PackHeader{}
    if err := r.In.Parse(r.Bytes, r.RespIndex, false); err != nil {
        return err
    }
    if r.In.Type != ipbus.Status {
        if err := r.In.Parse(r.Bytes, r.RespIndex, true); err != nil {
            return err
        }
    }
    return error(nil)

}

func CreateReqResp(req ipbus.Packet) ReqResp {
    b := make([]byte, 5000)
    return ReqResp{Out: req, Bytes: b}
}

func Clean(name string, errs chan ErrPack) {
    if r := recover(); r != nil {
        if err, ok := r.(error); ok {
            ep := MakeErrPack(err)
            fmt.Printf("Caught a panic: %s, %v\n", name, ep)
            errs <- ep
            fmt.Printf("Sent error pack to %v at %p\n", errs, &errs)
        } else {
            panic(r)
        }
    }
}

type ErrPack struct {
    Err error
    Stack []byte
}

func MakeErrPack(err error) ErrPack{
    stack := []byte{}
    if err != nil {
        stack = make([]byte, 1000000)
        n := runtime.Stack(stack, true)
        stack = stack[:n]
    }
    return ErrPack{err, stack}
}

func (ep ErrPack) String() string {
    return fmt.Sprintf("Error: %v,\n\n %s", ep.Err, ep.Stack)
}
