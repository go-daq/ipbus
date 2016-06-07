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
	/*
	dummy := newdummy(50002)
	if err := dummy.Start(); err != nil {
		t.Error(err)
	}

	if err := dummy.Stop(); err != nil {
		t.Error(err)
	}
	*/
	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}

// Test block read and block write.
func TestBlockReadWrite(t *testing.T) {

	if failunwritten {
		t.Errorf("Test function not yet implemented.")
	}
}

// Test that the library returns correct errors when going against target's permissions.
func TestPermissions(t *testing.T) {

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

