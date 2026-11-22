// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux

package perf_test

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/elastic/go-perf"
)

func TestPollScaling(t *testing.T) {
	t.Run("NoThreadPerWaiter", testPollNoThreadPerWaiter)
	t.Run("WakeupStress", testPollWakeupStress)
}

// testPollNoThreadPerWaiter checks that readers blocked on empty rings do
// not each hold an OS thread. System-wide monitoring opens one event per
// CPU, so a thread per waiter means a thread per CPU.
func testPollNoThreadPerWaiter(t *testing.T) {
	requires(t, paranoid(1), softwarePMU)

	const waiters = 64

	attr := new(perf.Attr)
	perf.Dummy.Configure(attr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := numThreads(t)
	errs := make(chan error, waiters)
	for i := 0; i < waiters; i++ {
		ev, err := perf.Open(attr, perf.CallingThread, perf.AnyCPU, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ev.Close()
		if err := ev.MapRing(); err != nil {
			t.Fatal(err)
		}
		go func() {
			_, err := ev.ReadRecord(ctx)
			errs <- err
		}()
	}

	// Give every reader time to block.
	time.Sleep(500 * time.Millisecond)
	if extra := numThreads(t) - before; extra > waiters/4 {
		t.Errorf("%d blocked readers created %d threads", waiters, extra)
	}

	cancel()
	timeout := time.After(10 * time.Second)
	for i := 0; i < waiters; i++ {
		select {
		case err := <-errs:
			if err != context.Canceled {
				t.Errorf("got %v, want context.Canceled", err)
			}
		case <-timeout:
			t.Fatalf("%d of %d readers did not return after cancel", waiters-i, waiters)
		}
	}
}

// testPollWakeupStress checks that a reader is woken for every record,
// whether the record arrives before or after the reader starts waiting.
func testPollWakeupStress(t *testing.T) {
	requires(t, paranoid(1), softwarePMU)

	attr := &perf.Attr{
		Options: perf.Options{
			Comm: true,
		},
	}
	perf.Dummy.Configure(attr)
	// Non-sample records only cause wakeups in watermark mode.
	attr.SetWakeupWatermark(1)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ev, err := perf.Open(attr, perf.CallingThread, perf.AnyCPU, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ev.Close()
	if err := ev.MapRing(); err != nil {
		t.Fatal(err)
	}

	name := []byte("perf-wakeup\x00")
	const iterations = 2000
	for i := 0; i < iterations; i++ {
		res := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, err := ev.ReadRecord(ctx)
			res <- err
		}()

		// Vary which side wins the race.
		switch i % 3 {
		case 1:
			runtime.Gosched()
		case 2:
			time.Sleep(time.Duration(i%50) * time.Microsecond)
		}

		// Renaming the monitored thread emits one PERF_RECORD_COMM.
		if err := unix.Prctl(unix.PR_SET_NAME, uintptr(unsafe.Pointer(&name[0])), 0, 0, 0); err != nil {
			t.Fatal(err)
		}
		if err := <-res; err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
}

func numThreads(t *testing.T) int {
	t.Helper()

	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(status, []byte("\n")) {
		if rest, ok := bytes.CutPrefix(line, []byte("Threads:")); ok {
			n, err := strconv.Atoi(string(bytes.TrimSpace(rest)))
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
	}
	t.Fatal("no Threads line in /proc/self/status")
	return 0
}
