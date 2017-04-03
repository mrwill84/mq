package stomp

import (
	"bufio"
	"io"
	"net"
	"time"

	"github.com/mrwill84/mq/logger"
)

const (
	bufferSize  = 32 << 10 // default buffer size 32KB
	bufferLimit = 32 << 15 // default buffer limit 1MB
)

var (
	never    time.Time
	deadline = time.Second * 5

	heartbeatTime = time.Second * 30
	heartbeatWait = time.Second * 60
)

type connPeer struct {
	conn net.Conn
	done chan bool

	reader   *bufio.Reader
	writer   *bufio.Writer
	incoming chan *Message
	outgoing chan *Message
}

// Conn creates a network-connected peer that reads and writes
// messages using net.Conn c.
func Conn(c net.Conn) Peer {
	p := &connPeer{
		reader:   bufio.NewReaderSize(c, bufferSize),
		writer:   bufio.NewWriterSize(c, bufferSize),
		incoming: make(chan *Message),
		outgoing: make(chan *Message),
		done:     make(chan bool),
		conn:     c,
	}

	go p.readInto(p.incoming)
	go p.writeFrom(p.outgoing)
	return p
}

func (c *connPeer) Receive() <-chan *Message {
	return c.incoming
}

func (c *connPeer) Send(message *Message) error {
	select {
	case <-c.done:
		return io.EOF
	default:
		c.outgoing <- message
		return nil
	}
}

func (c *connPeer) Addr() string {
	return c.conn.RemoteAddr().String()
}

func (c *connPeer) Close() error {
	return c.close()
}

func (c *connPeer) close() error {
	select {
	case <-c.done:
		return io.EOF
	default:
		close(c.done)
		close(c.incoming)
		close(c.outgoing)
		return nil
	}
}

func (c *connPeer) readInto(messages chan<- *Message) {
	defer c.close()

	for {
		// lim := io.LimitReader(c.conn, bufferLimit)
		// buf := bufio.NewReaderSize(lim, bufferSize)

		buf, err := c.reader.ReadBytes(0)
		if err != nil {
			break
		}
		if len(buf) == 1 {
			c.conn.SetReadDeadline(time.Now().Add(heartbeatWait))
			logger.Verbosef("stomp: received heart-beat")
			continue
		}

		msg := NewMessage()
		msg.Parse(buf[:len(buf)-1])

		select {
		case <-c.done:
			break
		default:
			messages <- msg
		}
	}
}

func (c *connPeer) writeFrom(messages <-chan *Message) {
	tick := time.NewTicker(time.Millisecond * 100).C
	heartbeat := time.NewTicker(heartbeatTime).C

loop:
	for {
		select {
		case <-c.done:
			break loop
		case <-heartbeat:
			logger.Verbosef("stomp: send heart-beat.")
			c.writer.WriteByte(0)
		case <-tick:
			c.conn.SetWriteDeadline(time.Now().Add(deadline))
			if err := c.writer.Flush(); err != nil {
				break loop
			}
			c.conn.SetWriteDeadline(never)
		case msg, ok := <-messages:
			if !ok {
				break loop
			}
			writeTo(c.writer, msg)
			c.writer.WriteByte(0)
			msg.Release()
		}
	}

	c.drain()
}

func (c *connPeer) drain() error {
	c.conn.SetWriteDeadline(time.Now().Add(deadline))
	for msg := range c.outgoing {
		writeTo(c.writer, msg)
		c.writer.WriteByte(0)
		msg.Release()
	}
	c.conn.SetWriteDeadline(never)
	c.writer.Flush()
	return c.conn.Close()
}
