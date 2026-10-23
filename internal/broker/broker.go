// Package broker manages the RabbitMQ connection: connecting with retry,
// reconnecting after a drop, publishing with confirms and consuming.
package broker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultConnectTimeout = 30 * time.Second
	defaultRetryInterval  = time.Second
	maxReconnectBackoff   = 30 * time.Second
	dialTimeout           = 5 * time.Second
	publishTimeout        = 5 * time.Second
	waitPollInterval      = 200 * time.Millisecond
)

// ErrNotConnected is returned while the connection to RabbitMQ is down.
var ErrNotConnected = errors.New("broker: not connected")

// SetupFunc declares the exchanges, queues and bindings the service needs. It
// runs on every (re)connection and must be idempotent.
type SetupFunc func(ch *amqp.Channel) error

// Broker is a RabbitMQ connection that reconnects by itself.
type Broker struct {
	url   string
	setup SetupFunc
	log   *slog.Logger

	mu   sync.RWMutex
	conn *amqp.Connection

	// pubMu serializes publishes on the single confirm-mode channel pubCh.
	pubMu sync.Mutex
	pubCh *amqp.Channel

	closing chan struct{}
	done    chan struct{}
}

// Connect connects to url, retrying for up to 30 seconds, runs setup and
// keeps the connection alive until Close.
func Connect(ctx context.Context, url string, setup SetupFunc, log *slog.Logger) (*Broker, error) {
	return connect(ctx, url, setup, log, defaultConnectTimeout, defaultRetryInterval)
}

func connect(
	ctx context.Context,
	url string,
	setup SetupFunc,
	log *slog.Logger,
	timeout, interval time.Duration,
) (*Broker, error) {
	if _, err := amqp.ParseURI(url); err != nil {
		// The parse error may echo parts of the URL, so it is not wrapped.
		return nil, errors.New("broker: invalid RABBITMQ_URL")
	}
	b := &Broker{
		url:     url,
		setup:   setup,
		log:     log,
		closing: make(chan struct{}),
		done:    make(chan struct{}),
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for attempt := 1; ; attempt++ {
		conn, err := b.dial()
		if err == nil {
			log.Info("message broker connected", "attempt", attempt)
			b.setConn(conn)
			go b.watch(conn)
			return b, nil
		}

		log.Warn("message broker not ready, retrying", "attempt", attempt, "error", err)

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("broker: could not connect within %s: %w", timeout, err)
		case <-time.After(interval):
		}
	}
}

// dial opens a connection and runs the setup on it.
func (b *Broker) dial() (*amqp.Connection, error) {
	conn, err := amqp.DialConfig(b.url, amqp.Config{Dial: amqp.DefaultDial(dialTimeout)})
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open setup channel: %w", err)
	}
	defer func() { _ = ch.Close() }()
	if err := b.setup(ch); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("declare topology: %w", err)
	}
	return conn, nil
}

// watch reconnects whenever conn is lost, until Close.
func (b *Broker) watch(conn *amqp.Connection) {
	defer close(b.done)
	for {
		lost := conn.NotifyClose(make(chan *amqp.Error, 1))
		select {
		case <-b.closing:
			return
		case amqpErr := <-lost:
			b.setConn(nil)
			b.log.Error("message broker connection lost, reconnecting", "error", amqpErr)
		}

		conn = b.reconnect()
		if conn == nil {
			return
		}
		b.setConn(conn)
		b.log.Info("message broker reconnected")
	}
}

// reconnect dials with a growing backoff until it succeeds or Close is called,
// in which case it returns nil.
func (b *Broker) reconnect() *amqp.Connection {
	backoff := defaultRetryInterval
	for {
		select {
		case <-b.closing:
			return nil
		case <-time.After(backoff):
		}

		conn, err := b.dial()
		if err == nil {
			return conn
		}
		b.log.Warn("message broker reconnect failed", "error", err, "next_attempt_in", backoff)
		backoff = min(backoff*2, maxReconnectBackoff)
	}
}

func (b *Broker) setConn(conn *amqp.Connection) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.conn = conn
}

// current returns the live connection, or nil while disconnected.
func (b *Broker) current() *amqp.Connection {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.conn == nil || b.conn.IsClosed() {
		return nil
	}
	return b.conn
}

// Ping reports ErrNotConnected while the connection is down.
func (b *Broker) Ping(context.Context) error {
	if b.current() == nil {
		return ErrNotConnected
	}
	return nil
}

// Publish sends msg and waits up to 5 seconds for the broker to confirm it.
func (b *Broker) Publish(ctx context.Context, exchange, key string, msg amqp.Publishing) error {
	b.pubMu.Lock()
	defer b.pubMu.Unlock()

	ch, err := b.publishChannel()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, key, false, false, msg)
	if err != nil {
		return fmt.Errorf("broker: publish: %w", err)
	}
	acked, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("broker: wait for confirm: %w", err)
	}
	if !acked {
		return errors.New("broker: message rejected by the broker")
	}
	return nil
}

// publishChannel returns the confirm-mode channel, reopening it after a
// connection or channel loss. The caller holds pubMu.
func (b *Broker) publishChannel() (*amqp.Channel, error) {
	if b.pubCh != nil && !b.pubCh.IsClosed() {
		return b.pubCh, nil
	}
	conn := b.current()
	if conn == nil {
		return nil, ErrNotConnected
	}
	ch, err := conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("broker: open publish channel: %w", err)
	}
	if err := ch.Confirm(false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("broker: enable publisher confirms: %w", err)
	}
	b.pubCh = ch
	return ch, nil
}

// Consume delivers the messages of queue to handle, one at a time, with
// prefetch messages buffered. It resubscribes after reconnections and returns
// when ctx is cancelled, after handle returns for the message in progress.
// Messages not yet acknowledged go back to the queue when the channel closes.
func (b *Broker) Consume(ctx context.Context, queue string, prefetch int, handle func(amqp.Delivery)) error {
	for {
		conn, err := b.waitConn(ctx)
		if err != nil {
			return nil
		}
		if err := b.consumeOnce(ctx, conn, queue, prefetch, handle); err != nil {
			b.log.Warn("consumer interrupted, resubscribing", "queue", queue, "error", err)
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}

func (b *Broker) consumeOnce(
	ctx context.Context,
	conn *amqp.Connection,
	queue string,
	prefetch int,
	handle func(amqp.Delivery),
) error {
	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open consumer channel: %w", err)
	}
	defer func() { _ = ch.Close() }()

	if err := ch.Qos(prefetch, 0, false); err != nil {
		return fmt.Errorf("set prefetch: %w", err)
	}
	const tag = "miner-consumer"
	deliveries, err := ch.Consume(queue, tag, false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume %s: %w", queue, err)
	}
	b.log.Info("consuming messages", "queue", queue)

	for {
		select {
		case <-ctx.Done():
			// Closing the channel with the deferred Close returns the
			// buffered, unacknowledged messages to the queue.
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("delivery channel closed")
			}
			handle(d)
		}
	}
}

// waitConn blocks until a connection is available or ctx is done.
func (b *Broker) waitConn(ctx context.Context) (*amqp.Connection, error) {
	ticker := time.NewTicker(waitPollInterval)
	defer ticker.Stop()
	for {
		if conn := b.current(); conn != nil {
			return conn, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// Close stops reconnecting and closes the connection.
func (b *Broker) Close() error {
	close(b.closing)
	<-b.done

	b.pubMu.Lock()
	if b.pubCh != nil {
		_ = b.pubCh.Close()
	}
	b.pubMu.Unlock()

	if conn := b.current(); conn != nil {
		return conn.Close()
	}
	return nil
}
