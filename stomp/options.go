package stomp

import (
	"math/rand"
	"strconv"
)

// MessageOption configures message options.
type MessageOption func(*Message)

// WithCredentials returns a MessageOption which sets credentials.
func WithCredentials(username, password string) MessageOption {
	return func(m *Message) {
		m.User = []byte(username)
		m.Pass = []byte(password)
	}
}

// WithHeader returns a MessageOption which sets a header.
func WithHeader(key, value string) MessageOption {
	return func(m *Message) {
		m.Header.Add(
			[]byte(key),
			[]byte(value),
		)
	}
}

// WithExpires returns a MessageOption configured with an expiration.
func WithExpires(exp int64) MessageOption {
	return func(m *Message) {
		m.Expires = exp
	}
}

// WithPrefetch returns a MessageOption configured with a prefetch count.
func WithPrefetch(prefetch int) MessageOption {
	return func(m *Message) {
		m.Prefetch = strconv.AppendInt(nil, int64(prefetch), 10)
	}
}

// WithReceipt returns a MessageOption configured with a receipt request.
func WithReceipt() MessageOption {
	return func(m *Message) {
		m.Receipt = strconv.AppendInt(nil, rand.Int63(), 10)
	}
}

// WithPersistence returns a MessageOption configured to persist.
func WithPersistence() MessageOption {
	return func(m *Message) {
		m.Persist = PersistTrue
	}
}

// WithRetain returns a MessageOption configured to retain the message.
func WithRetain(retain string) MessageOption {
	return func(m *Message) {
		m.Retain = []byte(retain)
	}
}

// WithSelector returns a MessageOption configured to filter messages
// using a sql-like evaluation string.
func WithSelector(selector string) MessageOption {
	return func(m *Message) {
		m.Selector = []byte(selector)
	}
}

// WithAck returns a MessageOption configured with an ack policy.
func WithAck(ack string) MessageOption {
	return func(m *Message) {
		m.Ack = []byte(ack)
	}
}
