package ssh

import (
	"context"
	"errors"
	"io"
	"net"
)

// streamLocalChannelOpenDirectMsg is a struct used for SSH_MSG_CHANNEL_OPEN message
// with "direct-streamlocal@openssh.com" string.
//
// See openssh-portable/PROTOCOL, section 2.4. connection: Unix domain socket forwarding
// https://github.com/openssh/openssh-portable/blob/master/PROTOCOL#L235
type streamLocalChannelOpenDirectMsg struct {
	socketPath string
	reserved0  string
	reserved1  uint32
}

// forwardedStreamLocalPayload is a struct used for SSH_MSG_CHANNEL_OPEN message
// with "forwarded-streamlocal@openssh.com" string.
type forwardedStreamLocalPayload struct {
	SocketPath string
	Reserved0  string
}

// streamLocalChannelForwardMsg is a struct used for SSH2_MSG_GLOBAL_REQUEST message
// with "streamlocal-forward@openssh.com"/"cancel-streamlocal-forward@openssh.com" string.
type streamLocalChannelForwardMsg struct {
	socketPath string
}

// ListenUnixContext is similar to ListenTCPContext but uses a Unix domain socket.
func (c *Client) ListenUnixContext(ctx context.Context, socketPath string) (net.Listener, error) {
	c.handleForwardsOnce.Do(c.handleForwards)
	m := streamLocalChannelForwardMsg{
		socketPath,
	}
	// send message
	ok, _, err := c.SendRequestWithContext(ctx, "streamlocal-forward@openssh.com", true, Marshal(&m))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("ssh: streamlocal-forward@openssh.com request denied by peer")
	}
	ch := c.forwards.add(&net.UnixAddr{Name: socketPath, Net: "unix"})

	return &unixListener{socketPath, c, ch}, nil
}

// ListenUnix simply calls ListenUnixContext with the default background context.
func (c *Client) ListenUnix(socketPath string) (net.Listener, error) {
	return c.ListenUnixContext(context.Background(), socketPath)
}

func (c *Client) dialStreamLocal(ctx context.Context, socketPath string) (Channel, error) {
	msg := streamLocalChannelOpenDirectMsg{
		socketPath: socketPath,
	}
	ch, in, err := c.OpenChannelWithContext(ctx, "direct-streamlocal@openssh.com", Marshal(&msg))
	if err != nil {
		return nil, err
	}
	go DiscardRequests(in)
	return ch, err
}

type unixListener struct {
	socketPath string

	conn *Client
	in   <-chan forward
}

// Accept waits for and returns the next connection to the listener.
func (l *unixListener) Accept() (net.Conn, error) {
	s, ok := <-l.in
	if !ok {
		return nil, io.EOF
	}
	ch, incoming, err := s.newCh.Accept()
	if err != nil {
		return nil, err
	}
	go DiscardRequests(incoming)

	return &chanConn{
		Channel: ch,
		laddr: &net.UnixAddr{
			Name: l.socketPath,
			Net:  "unix",
		},
		raddr: &net.UnixAddr{
			Name: "@",
			Net:  "unix",
		},
	}, nil
}

// CloseWithContext closes the listener.
func (l *unixListener) CloseWithContext(ctx context.Context) error {
	// this also closes the listener.
	l.conn.forwards.remove(&net.UnixAddr{Name: l.socketPath, Net: "unix"})
	m := streamLocalChannelForwardMsg{
		l.socketPath,
	}
	ok, _, err := l.conn.SendRequestWithContext(ctx, "cancel-streamlocal-forward@openssh.com", true, Marshal(&m))
	if err == nil && !ok {
		err = errors.New("ssh: cancel-streamlocal-forward@openssh.com failed")
	}
	return err
}

// Close simply calls CloseWithContext with the default background context.
func (l *unixListener) Close() error {
	return l.CloseWithContext(context.Background())
}

// Addr returns the listener's network address.
func (l *unixListener) Addr() net.Addr {
	return &net.UnixAddr{
		Name: l.socketPath,
		Net:  "unix",
	}
}
