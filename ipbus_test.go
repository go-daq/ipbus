package ipbus

import (
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"testing"
	"time"
)

const failunwritten = false

var dummy *dummyHardware
var dt = 60 * time.Second
var log *os.File
var target *Target
var nodummy *bool
var trenztarget *Target
var trenz *bool
var ipbusverbose *bool

func TestMain(m *testing.M) {
	ipbusverbose = flag.Bool("ipbusverbose", false, "Turn on verbosity of ipbus package")
	nodummy = flag.Bool("nodummyhardware", true, "Skip tests requiring dummy hardware.")
	trenz = flag.Bool("trenzhardware", false, "Enable tests against Trenz board.")
	flag.Parse()
	verbose = *ipbusverbose
	verbose = *ipbusverbose
	if !*nodummy {
		startdummy()
		defer dummy.Stop()
	}
	starttarget()
	if testing.Verbose() {
		fmt.Printf("Target regs: %v\n", target.Regs)
	}
	if *trenz {
		starttrenz()
	}
	code := m.Run()
	if !*nodummy {
		dummy.Kill <- true
	}
	time.Sleep(time.Second)
	os.Exit(code)
}

func starttrenz() {
	if trenztarget == nil {
		raddr, err := net.ResolveUDPAddr("udp4", "192.168.235.0:50001")
		if err != nil {
			panic(err)
		}
		conn, err := net.DialUDP("udp4", nil, raddr)
		if err != nil {
			panic(err)
		}
		t, err := New("dummy", "testdata/xml/trenz/top.xml", conn)
		if err != nil {
			panic(err)
		}
		trenztarget = &t
		fmt.Printf("Trenz board with registers:\n")
		regnames := make([]string, 0, len(trenztarget.Regs))
		for k := range trenztarget.Regs {
			regnames = append(regnames, k)
		}
		sort.Strings(regnames)
		for _, regname := range regnames {
			fmt.Printf("\t%v\n", trenztarget.Regs[regname])
		}
		if *ipbusverbose {
			trenztarget.hw.nverbose = 1
		}
	}

}

func startdummy() {
	if dummy == nil {
		cmd := exec.Command("killall", "DummyHardwareUdp.exe")
		err := cmd.Run()
		fmt.Printf("killall DummyHardwareUdp.exe: %v\n", err)
		d := newDummy(60001)
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
		raddr, err := net.ResolveUDPAddr("udp4", "localhost:60001")
		if err != nil {
			panic(err)
		}
		conn, err := net.DialUDP("udp4", nil, raddr)
		if err != nil {
			panic(err)
		}
		t, err := New("dummy", "testdata/xml/dummy_address.xml", conn)
		if err != nil {
			panic(err)
		}
		target = &t
		if *ipbusverbose {
			target.hw.nverbose = 1
		}
	}
}

// Ensure that creating a new target times out when there's no target present.
/*
func TestTimeout(t *testing.T) {

	conn, err := net.Dial("udp4", "locahost:60006")
	if err != nil {
		t.Fatal(err)
	}
	notarget, err := New("nodummy", "testdata/xml/dummy_address.xml", conn)
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
	if *nodummy {
		t.Skip()
	}
	testreg := Register{"REG", uint32(0x1), make([]string, 0), false, 1, make(map[string]msk)}
	/*
		testreg, ok := target.Regs["REG"]
		if !ok {
			t.Fatalf("Couldn't find test register 'REG' in dummy device description.")
		}
	*/
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

func TestRMWbits(t *testing.T) {
	if *nodummy {
		t.Skip()
	}
	testreg := Register{"REG", uint32(0x1), make([]string, 0), false, 1, make(map[string]msk)}
	/*
		testreg, ok := target.Regs["REG"]
		if !ok {
			t.Fatalf("Couldn't find test register 'REG' in dummy device description.")
		}
	*/
	testval := uint32(0xdeadbeef)
	andterm := uint32(0x0f0f0f0f)
	orterm := uint32(0xfedcba98)
	expectedresult := (testval & andterm) | orterm
	t.Logf("Writing single vale 0x%x to test register.", testval)
	t.Logf("testreg = %v\n", testreg)
	respchan := target.Write(testreg, []uint32{testval})
	target.Dispatch()
	resp := <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	t.Logf("RMWbits word in test register.")
	respchan = target.RMWbits(testreg, andterm, orterm)
	target.Dispatch()
	resp = <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	value := resp.Data[0]
	if value != testval {
		t.Fatalf("Reply from RMWbits value is 0x%x, expected 0x%x", value, testval)
	}
	t.Logf("Read word from test register.")
	respchan = target.Read(testreg, 1)
	target.Dispatch()
	resp = <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	value = resp.Data[0]
	if value != expectedresult {
		t.Fatalf("Read value 0x%x, expected 0x%x", value, expectedresult)
	}
	t.Logf("After RMWbits read word 0x%x", value)
}

func TestRMWsum(t *testing.T) {
	if *nodummy {
		t.Skip()
	}
	testreg := Register{"REG", uint32(0x1), make([]string, 0), false, 1, make(map[string]msk)}
	/*
		testreg, ok := target.Regs["REG"]
		if !ok {
			t.Fatalf("Couldn't find test register 'REG' in dummy device description.")
		}
	*/
	testval := uint32(0xdeadbeef)
	addend := uint32(0x0f0f0f0f)
	expectedresult := testval + addend
	t.Logf("Writing single vale 0x%x to test register.", testval)
	t.Logf("testreg = %v\n", testreg)
	respchan := target.Write(testreg, []uint32{testval})
	target.Dispatch()
	resp := <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	t.Logf("RMWsum word in test register.")
	respchan = target.RMWsum(testreg, addend)
	target.Dispatch()
	resp = <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	value := resp.Data[0]
	if value != testval {
		t.Fatalf("Reply from RMWsum value is 0x%x, expected 0x%x", value, testval)
	}
	t.Logf("Read word from test register.")
	respchan = target.Read(testreg, 1)
	target.Dispatch()
	resp = <-respchan
	if resp.Err != nil {
		t.Fatal(resp.Err)
	}
	value = resp.Data[0]
	if value != expectedresult {
		t.Fatalf("Read value 0x%x, expected 0x%x", value, expectedresult)
	}
	t.Logf("After RMWsum read word 0x%x", value)
}

// Test block read and block write.
func TestBlockReadWriteInc(t *testing.T) {
	if *nodummy {
		t.Skip()
	}

	testreg := Register{"MEM", uint32(0x100000), make([]string, 0), false, 268435456, make(map[string]msk)}
	//testreg, ok := target.Regs["MEM"]
	//if !ok {
	//t.Fatalf("Couldn't find test register 'MEM' in dummy device description.")
	//}
	nvals := 1000
	outdata := make([]uint32, nvals)
	indata := make([]uint32, 0, nvals)
	for i := 0; i < nvals; i++ {
		outdata[i] = uint32(i)
	}
	t.Logf("Writing %d words to test register.", nvals)
	t.Logf("testreg = %v\n", testreg)
	respchan := target.Write(testreg, outdata)
	target.Dispatch()
	ntrans := 0
	wordspertrans := make([]int, 0, 8)
	for resp := range respchan {
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}
		ntrans++
		wordspertrans = append(wordspertrans, len(resp.Data))
	}
	t.Logf("Send data in %d transactions: %v.", ntrans, wordspertrans)

	t.Logf("Reading %d words from test register.", nvals)
	respchan = target.Read(testreg, uint(nvals))
	target.Dispatch()
	ntrans = 0
	wordspertrans = make([]int, 0, 8)
	for resp := range respchan {
		if resp.Err != nil {
			t.Fatal(resp.Err)
		}
		indata = append(indata, resp.Data...)
		ntrans++
		wordspertrans = append(wordspertrans, len(resp.Data))
	}
	t.Logf("Received data in %d transactions: %v.", ntrans, wordspertrans)

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
}

// Test block read and block write of non-incrementing FIFO.
// The dummy hardware FIFO just reads back the last value it received.
func TestBlockReadWriteNonInc(t *testing.T) {
	if *nodummy {
		t.Skip()
	}

	testreg := Register{"FIFO", uint32(0x0100), make([]string, 0), true, 268435456, make(map[string]msk)}
	nvals := 350
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
			if indata[i] != outdata[nvals-1] {
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
}

// Test that the library returns correct errors when going against target's permissions.
func TestPermissions(t *testing.T) {
	if *nodummy {
		t.Skip()
	}
	/*
		cm, err := NewCM("testdata/xml/dummy_connections.xml")
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
	if *nodummy {
		b.Skip()
	}
	testreg := Register{"REG", uint32(0x1), make([]string, 0), false, 1, make(map[string]msk)}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		respchan := target.Read(testreg, 1)
		target.Dispatch()
		resp := <-respchan
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
	}

}

// Bench mark single word read.
func BenchmarkSingleReadTrenz(b *testing.B) {
	if !*trenz {
		b.Log("Skipping benchmark against Trenz board.")
		b.Skip()
	}
	testreg, ok := trenztarget.Regs["ctrl_reg.ctrl"]
	if !ok {
		b.Fatal("Failed to find reg `ctrl_reg.ctrl` in trenz register map.")
	}
	b.Log("Running test reading ctrl_reg.ctrl from Trenz board.")
	respchan := trenztarget.Read(testreg, 1)
	trenztarget.Dispatch()
	resp := <-respchan
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		respchan := trenztarget.Read(testreg, 1)
		trenztarget.Dispatch()
		resp := <-respchan
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
	}
}

// Bench mark single word read.
func BenchmarkMultiReadTrenz(b *testing.B) {
	if !*trenz {
		b.Log("Skipping benchmark against Trenz board.")
		b.Skip()
	}
	// Organise registers and masks
	fifo, ok := trenztarget.Regs["chan.fifo"]
	if !ok {
		b.Fatal("Failed to find reg `chan.fifo` in trenz register map.")
	}
	timingCsrCtrl, ok := trenztarget.Regs["timing.csr.ctrl"]
	if !ok {
		b.Fatal("Failed to find reg `timing.csr.ctrl` in trenz register map.")
	}
	chancap, ok := timingCsrCtrl.msks["chan_cap"]
	if !ok {
		b.Fatal("Failed to find mask `chan_cap` in `timing.csr.ctrl` register.")
	}
	ctrlregCtrl, ok := trenztarget.Regs["ctrl_reg.ctrl"]
	if !ok {
		b.Fatal("Failed to find reg `ctrl_reg.ctrl` in trenz register map.")
	}
	chansel, ok := ctrlregCtrl.msks["chan"]
	if !ok {
		b.Fatal("Failed to find mask `chan` in `ctrl_reg.ctrl` register.")
	}

	// Do first trigger and select channel 0
	triggerand := uint32(0xffffffff &^ chancap.value)
	selectand := uint32(0xffffffff &^ chansel.value)
	b.Log("Running test reading ctrl_reg.ctrl from Trenz board.")
	respchantrig1 := trenztarget.RMWbits(timingCsrCtrl, triggerand, chancap.value)
	respchantrig0 := trenztarget.RMWbits(timingCsrCtrl, triggerand, uint32(0))
	respchanselect := trenztarget.RMWbits(ctrlregCtrl, selectand, uint32(0))
	trenztarget.Dispatch()
	resp := <-respchantrig1
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	resp = <-respchantrig0
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	resp = <-respchanselect
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	b.Logf("First trigger and selected channel 0.")
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Read 2048 words then trigger again
		respchan := trenztarget.Read(fifo, 2048)
		respchantrig1 := trenztarget.RMWbits(timingCsrCtrl, triggerand, chancap.value)
		respchantrig0 := trenztarget.RMWbits(timingCsrCtrl, triggerand, uint32(0))
		trenztarget.Dispatch()
		ntrans := 0
		nword := 0
		for resp := range respchan {
			if resp.Err != nil {
				b.Fatal(resp.Err)
			}
			ntrans += 1
			nword += len(resp.Data)
		}
		//b.Logf("Received %d words in %d transactions.", nword, ntrans)
		resp = <-respchantrig1
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
		resp = <-respchantrig0
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
	}
}

// Bench mark single word read.
func BenchmarkMultiReadBTrenz(b *testing.B) {
	if !*trenz {
		b.Log("Skipping benchmark against Trenz board.")
		b.Skip()
	}
	// Organise registers and masks
	fifo, ok := trenztarget.Regs["chan.fifo"]
	if !ok {
		b.Fatal("Failed to find reg `chan.fifo` in trenz register map.")
	}
	timingCsrCtrl, ok := trenztarget.Regs["timing.csr.ctrl"]
	if !ok {
		b.Fatal("Failed to find reg `timing.csr.ctrl` in trenz register map.")
	}
	chancap, ok := timingCsrCtrl.msks["chan_cap"]
	if !ok {
		b.Fatal("Failed to find mask `chan_cap` in `timing.csr.ctrl` register.")
	}
	ctrlregCtrl, ok := trenztarget.Regs["ctrl_reg.ctrl"]
	if !ok {
		b.Fatal("Failed to find reg `ctrl_reg.ctrl` in trenz register map.")
	}
	chansel, ok := ctrlregCtrl.msks["chan"]
	if !ok {
		b.Fatal("Failed to find mask `chan` in `ctrl_reg.ctrl` register.")
	}

	// Do first trigger and select channel 0
	triggerand := uint32(0xffffffff &^ chancap.value)
	selectand := uint32(0xffffffff &^ chansel.value)
	b.Log("Running test reading ctrl_reg.ctrl from Trenz board.")
	respchantrig1 := trenztarget.RMWbits(timingCsrCtrl, triggerand, chancap.value)
	respchantrig0 := trenztarget.RMWbits(timingCsrCtrl, triggerand, uint32(0))
	respchanselect := trenztarget.RMWbits(ctrlregCtrl, selectand, uint32(0))
	trenztarget.Dispatch()
	resp := <-respchantrig1
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	resp = <-respchantrig0
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	resp = <-respchanselect
	if resp.Err != nil {
		b.Fatal(resp.Err)
	}
	b.Logf("First trigger and selected channel 0.")
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// Read 2048 words then trigger again
		respchan := trenztarget.ReadB(fifo, 2048)
		respchantrig1 := trenztarget.RMWbits(timingCsrCtrl, triggerand, chancap.value)
		respchantrig0 := trenztarget.RMWbits(timingCsrCtrl, triggerand, uint32(0))
		trenztarget.Dispatch()
		ntrans := 0
		nword := 0
		for resp := range respchan {
			if resp.Err != nil {
				b.Fatal(resp.Err)
			}
			ntrans += 1
			nword += len(resp.DataB)
		}
		//b.Logf("Received %d words in %d transactions.", nword, ntrans)
		resp = <-respchantrig1
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
		resp = <-respchantrig0
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
	}
}

// Bench mark single word write.
func BenchmarkSingleWrite(b *testing.B) {
	if *nodummy {
		b.Skip()
	}
	testreg := Register{"REG", uint32(0x1), make([]string, 0), false, 1, make(map[string]msk)}
	outdata := []uint32{0xdeadbeef}
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		respchan := target.Write(testreg, outdata)
		target.Dispatch()
		resp := <-respchan
		if resp.Err != nil {
			b.Fatal(resp.Err)
		}
	}
}

// Bench mark multi-packet block reads.
func BenchmarkBlockRead(b *testing.B) {
	if *nodummy {
		b.Skip()
	}

	testreg := Register{"MEM", uint32(0x100000), make([]string, 0), false, 262144, make(map[string]msk)}
	nword := 1000
	b.Logf("Writing %d bytes.", nword*4*b.N)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		respchan := target.Read(testreg, 1)
		target.Dispatch()
		for resp := range respchan {
			if resp.Err != nil {
				b.Fatal(resp.Err)
			}
		}

	}

}

// Bench mark multi-packet block writes.
func BenchmarkBlockWrite(b *testing.B) {
	if *nodummy {
		b.Skip()
	}

	testreg := Register{"MEM", uint32(0x100000), make([]string, 0), false, 262144, make(map[string]msk)}
	nword := 1000
	outdata := make([]uint32, nword)
	for i := 0; i < nword; i++ {
		outdata[i] = uint32(i)
	}
	b.Logf("Writing %d bytes.", nword*4*b.N)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		respchan := target.Write(testreg, outdata)
		target.Dispatch()
		for resp := range respchan {
			if resp.Err != nil {
				b.Fatal(resp.Err)
			}
		}
	}
}
