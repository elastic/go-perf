// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux

package perf

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Link represents a BPF link attached to a perf event.
type Link struct {
	fd     int
	closed bool
}

type bpfLinkCreatePerfEventAttr struct {
	ProgFD     uint32
	TargetFD   uint32
	AttachType uint32
	Flags      uint32
	BPFCookie  uint64
}

// LinkOptions contains options for creating a BPF link.
type LinkOptions struct {
	// Cookie is accessible to the BPF program via bpf_get_attach_cookie().
	Cookie uint64
}

// SetBPFLink attaches a BPF program to ev using the BPF link mechanism.
// progfd is the file descriptor of the BPF program to attach.
// opts may be nil for default options.
func (ev *Event) SetBPFLink(progfd int, opts *LinkOptions) (*Link, error) {
	if err := ev.ok(); err != nil {
		return nil, err
	}

	perffd, err := ev.FD()
	if err != nil {
		return nil, err
	}

	var cookie uint64
	if opts != nil {
		cookie = opts.Cookie
	}

	attr := bpfLinkCreatePerfEventAttr{
		ProgFD:     uint32(progfd),
		TargetFD:   uint32(perffd),
		AttachType: unix.BPF_PERF_EVENT,
		Flags:      0,
		BPFCookie:  cookie,
	}

	fd, err := bpfLinkCreate(&attr)
	if err != nil {
		return nil, fmt.Errorf("BPF_LINK_CREATE: %w", err)
	}

	return &Link{fd: fd}, nil
}

// FD returns the file descriptor of the BPF link.
func (l *Link) FD() int {
	return l.fd
}

// Close closes the BPF link, detaching the BPF program from the perf event.
func (l *Link) Close() error {
	if l == nil || l.closed {
		return nil
	}
	l.closed = true
	return unix.Close(l.fd)
}

func bpfLinkCreate(attr *bpfLinkCreatePerfEventAttr) (int, error) {
	fd, _, errno := unix.Syscall(
		unix.SYS_BPF,
		unix.BPF_LINK_CREATE,
		uintptr(unsafe.Pointer(attr)),
		unsafe.Sizeof(*attr),
	)
	if errno != 0 {
		return -1, os.NewSyscallError("bpf", errno)
	}
	return int(fd), nil
}

// ErrBPFLinkNotSupported is returned when BPF link creation is not supported.
var ErrBPFLinkNotSupported = errors.New("BPF link not supported (requires Linux 5.15+)")

// IsBPFLinkSupported returns true if the kernel supports BPF links for
// perf events.
func IsBPFLinkSupported() bool {
	attr := bpfLinkCreatePerfEventAttr{
		ProgFD:     ^uint32(0),
		TargetFD:   ^uint32(0),
		AttachType: unix.BPF_PERF_EVENT,
	}
	_, err := bpfLinkCreate(&attr)
	if err == nil {
		return true
	}
	return errors.Is(err, unix.EBADF)
}
