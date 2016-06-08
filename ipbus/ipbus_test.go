package ipbus

import (
	//"math/rand"
	"testing"
)

const failunwritten = false

// Ensure that creating a new target times out when there's no target present.
func TestTimeout(t *testing.T) {

	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}

// Test single word read and write.
func TestSingleReadWrite(t *testing.T) {
	dummy := newdummy(60001)
	t.Logf("Starting dummy hardware...")
	if err := dummy.Start(); err != nil {
		t.Error(err)
	}
	t.Logf("Started dummy hardware.")
	defer dummy.Stop()
	cm, err := NewCM("xml/dummy_connections.xml")
	if err != nil {
		t.Fatal(err)
	}
	//t.Logf("cm.Devices: %v", cm.Devices)
	target, err := cm.Target("dummy.udp2")
	if err != nil {
		t.Fatal(err)
	}
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

	/*
		if failunwritten {
			t.Errorf("Test function not yet implemented.")
		}
	*/
}

// Test block read and block write.
func TestBlockReadWrite(t *testing.T) {

	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}

// Test that the library returns correct errors when going against target's permissions.
func TestPermissions(t *testing.T) {
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
