package ssh

import (
	"bufio"
	"fmt"
	"io"
	"time"
)

var unsupported = fmt.Errorf("unsupported")

// ReaderWithDeadlineSetter is a io.Reader that also supports SetReadDeadline().
type readerWithDeadlineSetter interface {
	io.Reader
	SetReadDeadline(t time.Time) error
}

// WriterWithDeadlineSetter is a io.Writer that also supports SetWriteDeadline().
type writerWithDeadlineSetter interface {
	io.Writer
	SetWriteDeadline(t time.Time) error
}

type bufReaderWithDeadline struct {
	*bufio.Reader
	UnderlyingReader io.Reader
}

func newBufReaderWithDeadline(r io.Reader) *bufReaderWithDeadline {
	return &bufReaderWithDeadline{
		bufio.NewReader(r),
		r,
	}
}

func (r *bufReaderWithDeadline) SetReadDeadline(t time.Time) error {
	rs, ok := r.UnderlyingReader.(readerWithDeadlineSetter)
	if !ok {
		return unsupported
	}
	return rs.SetReadDeadline(t)
}

type bufWriterWithDeadline struct {
	*bufio.Writer
	UnderlyingWriter io.Writer
}

func newBufWriterWithDeadline(w io.Writer) *bufWriterWithDeadline {
	return &bufWriterWithDeadline{
		bufio.NewWriter(w),
		w,
	}
}

func (r *bufWriterWithDeadline) SetWriteDeadline(t time.Time) error {
	rs, ok := r.UnderlyingWriter.(writerWithDeadlineSetter)
	if !ok {
		return unsupported
	}
	return rs.SetWriteDeadline(t)
}

func tryCancelReader(r io.Reader) error {
	rs, ok := r.(readerWithDeadlineSetter)
	if ok {
		return rs.SetReadDeadline(time.Unix(0, 0))
	}
	return unsupported
}

func tryCancelWriter(w io.Writer) error {
	ws, ok := w.(writerWithDeadlineSetter)
	if ok {
		return ws.SetWriteDeadline(time.Unix(0, 0))
	}
	return unsupported
}
