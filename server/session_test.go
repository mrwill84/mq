package server

import (
	"bytes"
	"testing"

	"github.com/mrwill84/mq/stomp"
)

func Test_session_subscribe(t *testing.T) {
	sess := requestSession()
	defer sess.release()

	msg := stomp.NewMessage()
	msg.Dest = []byte("/topic/test")
	msg.ID = []byte("123")
	msg.Prefetch = []byte("2")
	msg.Selector = []byte("ram > 2")
	defer msg.Release()

	sub := sess.subs(msg)
	if sub.prefetch != 2 {
		t.Errorf("expected subscription prefix copied from message")
	}
	if !bytes.Equal(sub.id, []byte("123")) {
		t.Errorf("expected subscription id correctly set, got %d", sub.id)
	}
	if sub.session != sess {
		t.Errorf("expect session attached to subscription")
	}
	if sess.sub["123"] != sub {
		t.Errorf("expect subscription tracked in session map")
	}
	if sub.selector == nil {
		t.Errorf("expect subscription sql selector successfully parsed")
	}

	sess.unsub(sub)
	if len(sub.id) != 0 {
		t.Errorf("expected subscription reset")
	}
	if sess.sub["1"] != nil {
		t.Errorf("expect subscription removed from session")
	}
}

func Test_session_reset(t *testing.T) {
	sess := &session{
		peer: nil,
		sub: map[string]*subscription{
			"0": &subscription{},
		},
		ack: map[string]*stomp.Message{
			"0": &stomp.Message{},
		},
	}
	sess.reset()

	if sess.peer != nil {
		t.Errorf("expect session transport reset")
	}
	if len(sess.sub) != 0 {
		t.Errorf("expect session subscription map reset")
	}
	if len(sess.ack) != 0 {
		t.Errorf("expect session acknowledgement map reset")
	}
}

func Test_session_send(t *testing.T) {
	a, b := stomp.Pipe()

	s := requestSession()
	s.peer = a

	sent := stomp.NewMessage()
	s.send(sent)

	recv := <-b.Receive()
	if sent != recv {
		t.Errorf("expect session.send to send message to peer")
	}

	sent.Release()
	s.release()
}

func Test_session_pool(t *testing.T) {
	s := requestSession()
	if s == nil {
		t.Errorf("expected session from pool")
	}
	s.release()
}
