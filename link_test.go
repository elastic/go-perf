// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package perf_test

import (
	"errors"
	"runtime"
	"testing"

	"golang.org/x/sys/unix"

	"github.com/elastic/go-perf"
)

func TestIsBPFLinkSupported(t *testing.T) {
	supported := perf.IsBPFLinkSupported()
	t.Logf("BPF link supported: %v", supported)
}

func TestSetBPFLink(t *testing.T) {
	requires(t, paranoid(1), softwarePMU)

	if !perf.IsBPFLinkSupported() {
		t.Skip("BPF link not supported on this kernel")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	attr := new(perf.Attr)
	perf.PageFaults.Configure(attr)
	attr.SetSamplePeriod(1)
	attr.SampleFormat.IP = true

	ev, err := perf.Open(attr, perf.CallingThread, perf.AnyCPU, nil)
	if err != nil {
		t.Fatalf("failed to open event: %v", err)
	}
	defer ev.Close()

	_, err = ev.SetBPFLink(-1, &perf.LinkOptions{Cookie: 0x1234})
	if err == nil {
		t.Fatal("expected error for invalid program fd")
	}
	if !errors.Is(err, unix.EBADF) {
		t.Logf("got error: %v (expected EBADF)", err)
	}
}

func TestSetBPFLinkClosedEvent(t *testing.T) {
	requires(t, paranoid(1), softwarePMU)

	if !perf.IsBPFLinkSupported() {
		t.Skip("BPF link not supported on this kernel")
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	attr := new(perf.Attr)
	perf.PageFaults.Configure(attr)
	attr.SetSamplePeriod(1)

	ev, err := perf.Open(attr, perf.CallingThread, perf.AnyCPU, nil)
	if err != nil {
		t.Fatalf("failed to open event: %v", err)
	}
	ev.Close()

	_, err = ev.SetBPFLink(0, nil)
	if err == nil {
		t.Fatal("expected error for closed event")
	}
}

func TestLinkClose(t *testing.T) {
	var l *perf.Link
	if err := l.Close(); err != nil {
		t.Errorf("Close on nil link returned error: %v", err)
	}
}
