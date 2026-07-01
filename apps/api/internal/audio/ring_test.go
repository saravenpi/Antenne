package audio

import (
	"sync"
	"testing"
	"time"
)

func TestRingFIFO(t *testing.T) {
	r := newRing[int](4)
	for i := 0; i < 4; i++ {
		if !r.push(i) {
			t.Fatalf("push %d failed", i)
		}
	}
	if r.len() != 4 {
		t.Fatalf("len = %d, want 4", r.len())
	}
	for i := 0; i < 4; i++ {
		v, ok := r.pop()
		if !ok || v != i {
			t.Fatalf("pop = %d,%v want %d,true", v, ok, i)
		}
	}
	if _, ok := r.pop(); ok {
		t.Fatal("pop on empty ring returned ok")
	}
}

func TestRingWrapAround(t *testing.T) {
	r := newRing[int](3)
	r.push(1)
	r.push(2)
	r.pop() // drop 1, head advances
	r.push(3)
	r.push(4) // wraps into the slot freed by popping 1
	got := []int{}
	for {
		v, ok := r.pop()
		if !ok {
			break
		}
		got = append(got, v)
	}
	want := []int{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// A full ring must block the producer until the consumer pops, never dropping.
func TestRingBackpressure(t *testing.T) {
	r := newRing[int](2)
	r.push(1)
	r.push(2)

	pushed := make(chan struct{})
	go func() {
		r.push(3) // should block until a pop frees a slot
		close(pushed)
	}()

	select {
	case <-pushed:
		t.Fatal("push returned while ring was full")
	case <-time.After(20 * time.Millisecond):
	}

	if v, ok := r.pop(); !ok || v != 1 {
		t.Fatalf("pop = %d,%v want 1,true", v, ok)
	}

	select {
	case <-pushed:
	case <-time.After(time.Second):
		t.Fatal("push did not unblock after pop")
	}
}

// close must wake a blocked producer and make push report failure.
func TestRingCloseWakesProducer(t *testing.T) {
	r := newRing[int](1)
	r.push(1)

	result := make(chan bool, 1)
	go func() { result <- r.push(2) }()

	time.Sleep(10 * time.Millisecond)
	r.close()

	select {
	case ok := <-result:
		if ok {
			t.Fatal("push after close returned true")
		}
	case <-time.After(time.Second):
		t.Fatal("close did not wake blocked producer")
	}

	// Buffered frames remain poppable after close (tail drain).
	if v, ok := r.pop(); !ok || v != 1 {
		t.Fatalf("tail pop = %d,%v want 1,true", v, ok)
	}
}

func TestRingFlush(t *testing.T) {
	r := newRing[int](8)
	for i := 0; i < 5; i++ {
		r.push(i)
	}
	r.flush()
	if r.len() != 0 {
		t.Fatalf("len after flush = %d, want 0", r.len())
	}
	// Ring is reusable after a flush.
	r.push(42)
	if v, ok := r.pop(); !ok || v != 42 {
		t.Fatalf("pop after flush = %d,%v want 42,true", v, ok)
	}
}

// Exercise the concurrent producer/consumer path under the race detector.
func TestRingConcurrent(t *testing.T) {
	r := newRing[int](16)
	const n = 10000
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			r.push(i)
		}
		r.close()
	}()

	got := 0
	next := 0
	for {
		v, ok := r.pop()
		if !ok {
			if got == n {
				break
			}
			time.Sleep(time.Millisecond)
			continue
		}
		if v != next {
			t.Fatalf("out of order: got %d want %d", v, next)
		}
		next++
		got++
	}
	wg.Wait()
	if got != n {
		t.Fatalf("received %d frames, want %d", got, n)
	}
}
