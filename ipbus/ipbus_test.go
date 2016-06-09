package ipbus

import (
	//"math/rand"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	ipbusverbose := flag.Bool("ipbusverbose", false, "Turn on verbosity of ipbus package")
	flag.Parse()
	verbose = *ipbusverbose
	startdummy()
	defer dummy.Stop()
	starttarget()
	if testing.Verbose() {
		fmt.Printf("Target regs: %v\n", target.Regs)
	}
	code := m.Run()
	dummy.Kill <-true
	time.Sleep(time.Second)
	os.Exit(code)
}

const failunwritten = false
var dummy *DummyHardware
var dt = 60 * time.Second
var log *os.File
var target *Target

func startdummy() {
	if dummy == nil {
		cmd := exec.Command("killall", "DummyHardwareUdp.exe")
		err := cmd.Run()
		fmt.Printf("killall DummyHardwareUdp.exe: %v\n", err)
		d := NewDummy(60001)
		dummy = &d
		log, err := os.Create("dummyhardwarelog.txt")
		if err != nil {
			panic(err)
		}
		go dummy.Run(dt, log)
		time.Sleep(time.Second)
	}
}

func starttarget() {
	if target == nil {
		conn, err := net.Dial("udp4", "localhost:60001")
		if err != nil {
			panic(err)
		}
		t, err := New("dummy", "xml/dummy_address.xml", conn)
		if err != nil {
			panic(err)
		}
		target = &t
	}
}

// Ensure that creating a new target times out when there's no target present.
/*
func TestTimeout(t *testing.T) {

	conn, err := net.Dial("udp4", "locahost:60006")
	if err != nil {
		t.Fatal(err)
	}
	notarget, err := New("nodummy", "xml/dummy_address.xml", conn)
	if err != nil {
		t.Fatal(err)
	}
	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}
*/

// Test single word read and write.
func TestSingleReadWrite(t *testing.T) {
	testreg, ok := target.Regs["REG"]
	if !ok {
		t.Fatalf("Couldn't find test register 'REG' in dummy device description.")
	}
	testval := uint32(0xdeadbeef)
	t.Logf("Writing single vale 0x%x to test register.", testval)
	t.Logf("testreg = %v\n", testreg)
	respchan := target.Write(testreg, []uint32{testval})
	target.Dispatch()
	resp := <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}

	t.Logf("Reading single word from test register.")
	respchan = target.Read(testreg, 1)
	target.Dispatch()
	resp = <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	value := resp.Data[0]
	if value != testval {
		t.Fatalf("Read value 0x%x, expected 0x%x", value, testval)
	}
	t.Logf("Read word 0x%x", value)
}

// Test block read and block write.
func TestBlockReadWrite(t *testing.T) {

	testreg, ok := target.Regs["MEM"]
	if !ok {
		t.Fatalf("Couldn't find test register 'MEM' in dummy device description.")
	}
	nvals := 300
	outdata := make([]uint32, nvals)
	indata := make([]uint32, 0, nvals)
	for i := 0; i < nvals; i++ {
		outdata[i] = uint32(i)
	}
	t.Logf("Writing %d words to test register.", nvals)
	t.Logf("testreg = %v\n", testreg)
	respchan := target.Write(testreg, outdata)
	target.Dispatch()
	npacks := 0
	for resp := range respchan {
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}
		npacks++
	}
	t.Logf("Send data in %d packets.", npacks)

	t.Logf("Reading %d words from test register.", nvals)
	respchan = target.Read(testreg, uint(nvals))
	target.Dispatch()
	for resp := range respchan {
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}
		indata = append(indata, resp.Data...)
	}
	if len(indata) == nvals {
		nwrong := 0
		for i := 0; i < nvals; i++ {
			if outdata[i] != indata[i] {
				if nwrong < 3 {
					t.Errorf("indata[%d] = 0x%x, expected 0x%x", i, indata[i], outdata[i])
				} else {
					t.Errorf("More than 3 wrong values...")
					break
				}
				nwrong++
			}
		}
	} else {
		t.Errorf("Expected %d values, received %v", nvals, len(indata))
	}
	/*
	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
	*/
}

// Test that the library returns correct errors when going against target's permissions.
func TestPermissions(t *testing.T) {
	/*
	cm, err := NewCM("xml/dummy_connections.xml")
	if err != nil {
		t.Fatal(err)
	}
	target, err := cm.Target("dummy.udp2")
	if err != nil {
		t.Fatal(err)
	}
	writeonlyreg, ok := target.Regs["REG_WRITE_ONLY"]
	if !ok {
		t.Fatalf("Failed to find `REG_WRITE_ONLY` register.")
	}
	readonlyreg, ok := target.Regs["REG_READ_ONLY"]
	if !ok {
		t.Fatalf("Failed to find `REG_READ_ONLY` register.")
	}
	t.Logf("Read-only: %v, write-only: %v\n", readonlyreg, writeonlyreg)

	t.Log("Tring to read from a write-only regiser.")
	*/
	/*
		respchan := target.Read(writeonlyreg, 1)
		target.Dispatch()
		resp := <-respchan
		if resp.Err == nil || resp.Code != BusReadError {
			t.Errorf("Expected permission fail when reading write-only register. Err = %v, code = %v.\n", resp.Err, resp.Code)
		}
	*/
	t.Log("Trying to write to a read-only register.")
	/*
		respchan = target.Write(readonlyreg, []uint32{0})
		target.Dispatch()
		resp = <-respchan
		if resp.Err == nil || resp.Code != BusWriteError {
			t.Errorf("Expected permission fail when writing read-only register. Err = %v, code = %v.\n", resp.Err, resp.Code)
		}
	*/

	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}

// Bench mark single word read.
func BenchmarkSingleRead(b *testing.B) {

	b.ResetTimer()
	for n := 0; n < b.N; n++ {

	}

}

// Bench mark single word write.
func BenchmarkSingleWrite(b *testing.B) {
	/*
		target := Target{}
		reg := Register{}
		nvals := 1024
		data := make([]uint32, nvals)
		for i := 0; i < nvals; i++ {
			data[i] = uint32(rand.Int31())
		}
		datum := make([]uint32, 1)
		b.ResetTimer()
		for n := 0; n < b.N; n++ {
			datum[0] = data[n % nvals]
			resp := target.Write(reg, datum)
			target.Dispatch()
			rep := <-resp
			if rep.Err != nil {
				b.Log(rep.Err)
			}
		}
	*/

}

// Bench mark multi-packet block reads.
func BenchmarkBlockRead(b *testing.B) {

	b.ResetTimer()
	for n := 0; n < b.N; n++ {

	}

}

// Bench mark multi-packet block writes.
func BenchmarkBlockWrite(b *testing.B) {

	b.ResetTimer()
	for n := 0; n < b.N; n++ {

	}

}
