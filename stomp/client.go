package stomp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/mrwill84/mq/logger"
	"github.com/mrwill84/mq/stomp/dialer"
)

// Client defines a client connection to a STOMP server.
type Client struct {
	mu sync.Mutex

	peer Peer
	subs map[string]Handler
	wait map[string]chan struct{}
	done chan error

	seq int64

	skipVerify      bool
	readBufferSize  int
	writeBufferSize int
	timeout         time.Duration
}

// New returns a new STOMP client using the given connection.
func New(peer Peer) *Client {
	return &Client{
		peer: peer,
		subs: make(map[string]Handler),
		wait: make(map[string]chan struct{}),
		done: make(chan error, 1),
	}
}

// Dial creates a client connection to the given target.
func Dial(target string) (*Client, error) {
	conn, err := dialer.Dial(target)
	if err != nil {
		return nil, err
	}
	return New(Conn(conn)), nil
}

// Send sends the data to the given destination.
func (c *Client) Send(dest string, data []byte, opts ...MessageOption) error {
	m := NewMessage()
	m.Method = MethodSend
	m.Dest = []byte(dest)
	m.Body = data
	m.Apply(opts...)
	return c.sendMessage(m)
}

// SendJSON sends the JSON encoding of v to the given destination.
func (c *Client) SendJSON(dest string, v interface{}, opts ...MessageOption) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	opts = append(opts,
		WithHeader("content-type", "application/json"),
	)
	return c.Send(dest, data, opts...)
}

// Subscribe subscribes to the given destination.
func (c *Client) Subscribe(dest string, handler Handler, opts ...MessageOption) (id []byte, err error) {
	id = c.incr()

	m := NewMessage()
	m.Method = MethodSubscribe
	m.ID = id
	m.Dest = []byte(dest)
	m.Apply(opts...)

	c.mu.Lock()
	c.subs[string(id)] = handler
	c.mu.Unlock()

	err = c.sendMessage(m)
	if err != nil {
		c.mu.Lock()
		delete(c.subs, string(id))
		c.mu.Unlock()
		return
	}
	return
}

// Unsubscribe unsubscribes to the destination.
func (c *Client) Unsubscribe(id []byte, opts ...MessageOption) error {
	c.mu.Lock()
	delete(c.subs, string(id))
	c.mu.Unlock()

	m := NewMessage()
	m.Method = MethodUnsubscribe
	m.ID = id
	m.Apply(opts...)

	return c.sendMessage(m)
}

// Ack acknowledges the messages with the given id.
func (c *Client) Ack(id []byte, opts ...MessageOption) error {
	m := NewMessage()
	m.Method = MethodAck
	m.ID = id
	m.Apply(opts...)

	return c.sendMessage(m)
}

// Nack negative-acknowledges the messages with the given id.
func (c *Client) Nack(id []byte, opts ...MessageOption) error {
	m := NewMessage()
	m.Method = MethodNack
	m.ID = id
	m.Apply(opts...)

	return c.peer.Send(m)
}

// Connect opens the connection and establishes the session.
func (c *Client) Connect(opts ...MessageOption) error {
	m := NewMessage()
	m.Proto = STOMP
	m.Method = MethodStomp
	m.Apply(opts...)
	if err := c.sendMessage(m); err != nil {
		return err
	}

	m, ok := <-c.peer.Receive()
	if !ok {
		return io.EOF
	}
	defer m.Release()

	if !bytes.Equal(m.Method, MethodConnected) {
		return fmt.Errorf("stomp: inbound message: unexpected method, want connected")
	}
	go c.listen()
	return nil
}

// Disconnect terminates the session and closes the connection.
func (c *Client) Disconnect() error {
	m := NewMessage()
	m.Method = MethodDisconnect
	c.sendMessage(m)
	return c.peer.Close()
}

// Done returns a channel
func (c *Client) Done() <-chan error {
	return c.done
}

func (c *Client) incr() []byte {
	c.mu.Lock()
	i := c.seq
	c.seq++
	c.mu.Unlock()
	return strconv.AppendInt(nil, i, 10)
}

func (c *Client) listen() {
	defer func() {
		if r := recover(); r != nil {
			logger.Warningf("stomp client: recover panic: %s", r)
			c.done <- r.(error)
		}
	}()

	for {
		m, ok := <-c.peer.Receive()
		if !ok {
			c.done <- io.EOF
			return
		}

		switch {
		case bytes.Equal(m.Method, MethodMessage):
			c.handleMessage(m)
		case bytes.Equal(m.Method, MethodRecipet):
			c.handleReceipt(m)
		default:
			logger.Noticef("stomp client: unknown message type: %s",
				string(m.Method),
			)
		}
	}
}

func (c *Client) handleReceipt(m *Message) {
	c.mu.Lock()
	receiptc, ok := c.wait[string(m.Receipt)]
	c.mu.Unlock()
	if !ok {
		logger.Noticef("stomp client: unknown read receipt: %s",
			string(m.Receipt),
		)
		return
	}
	receiptc <- struct{}{}
}

func (c *Client) handleMessage(m *Message) {
	c.mu.Lock()
	handler, ok := c.subs[string(m.Subs)]
	c.mu.Unlock()
	if !ok {
		logger.Noticef("stomp client: subscription not found: %s",
			string(m.Subs),
		)
		return
	}
	handler.Handle(m)
}

func (c *Client) sendMessage(m *Message) error {
	if len(m.Receipt) == 0 {
		return c.peer.Send(m)
	}

	receiptc := make(chan struct{}, 1)
	c.wait[string(m.Receipt)] = receiptc

	defer func() {
		delete(c.wait, string(m.Receipt))
	}()

	err := c.peer.Send(m)
	if err != nil {
		return err
	}

	select {
	case <-receiptc:
		return nil
	}
}
