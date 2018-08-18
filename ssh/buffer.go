// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ssh

import (
	"context"
	"io"
	"sync"
)

// buffer provides a linked list buffer for data exchange
// between producer and consumer. Theoretically the buffer is
// of unlimited capacity as it does no allocation of its own.
type buffer struct {
	// protects concurrent access to head, tail and closed
	opCh chan func()

	// notifies the waiting reader that the pending ops have been completed.
	finCh chan struct{}

	head *element // the buffer that will be read first
	tail *element // the buffer that will be read last

	mu     sync.Mutex
	closed bool
}

// An element represents a single link in a linked list.
type element struct {
	buf  []byte
	next *element
}

// newBuffer returns an empty buffer that is not closed.
func newBuffer() *buffer {
	e := new(element)
	b := &buffer{
		opCh:  make(chan func()),
		finCh: make(chan struct{}),
		head:  e,
		tail:  e,
	}
	go func() {
		for bop := range b.opCh {
			bop()
		}
		close(b.finCh)
	}()
	return b
}

// write makes buf available for Read to receive.
// buf must not be modified after the call to write.
func (b *buffer) write(buf []byte) {
	doneCh := make(chan struct{})
	b.opCh <- func() {
		e := &element{buf: buf}
		b.tail.next = e
		b.tail = e
		doneCh <- struct{}{}
	}
	<-doneCh
}

// eof closes the buffer. Reads from the buffer once all
// the data has been consumed will receive io.EOF.
func (b *buffer) eof() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.closed {
		b.closed = true
		close(b.opCh)
	}
}

// read reads data from the internal buffer. lock must be held before the invocation.
func (b *buffer) read(buf []byte) (n int) {
	for len(buf) > 0 {
		// if there is data in b.head, copy it
		if len(b.head.buf) > 0 {
			r := copy(buf, b.head.buf)
			buf, b.head.buf = buf[r:], b.head.buf[r:]
			n += r
			continue
		}
		// if there is a next buffer, make it the head
		if len(b.head.buf) == 0 && b.head != b.tail {
			b.head = b.head.next
			continue
		}
		break
	}
	return n
}

// Read reads data from the internal buffer in buf.  Reads will block
// if no data is available, or until the buffer is closed.
func (b *buffer) ReadWithContext(ctx context.Context, buf []byte) (n int, err error) {
outer:
	for len(buf) > 0 {
		b.mu.Lock()
		closed := b.closed
		var ch chan int
		if !closed {
			ch = make(chan int, 1)
			b.opCh <- func() {
				ch <- b.read(buf)
			}
		}
		b.mu.Unlock()

		// out of buffers, wait for producer
		if closed {
			select {
			case <-b.finCh:
			case <-ctx.Done():
				err = context.Canceled
				break outer
			}
			n = b.read(buf)
		} else {
			select {
			case n = <-ch:
			case <-ctx.Done():
				err = context.Canceled
				break outer
			}
		}
		// if at least one byte has been copied, return
		if n > 0 {
			break
		}

		// if nothing was read, and there is nothing outstanding
		// check to see if the buffer is closed.
		if closed {
			err = io.EOF
			break
		}
	}
	return
}
