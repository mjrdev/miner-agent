package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
)

const (
	prefetch          = 10
	retryDelay        = time.Second
	processingTimeout = 30 * time.Second
)

// Envelope is the message published for every accepted webhook.
type Envelope struct {
	Provider   string          `json:"provider"`
	DeliveryID string          `json:"delivery_id"`
	ReceivedAt time.Time       `json:"received_at"`
	Payload    json.RawMessage `json:"payload"`
}

type publisher interface {
	Publish(ctx context.Context, exchange, key string, msg amqp.Publishing) error
}

// Publisher enqueues webhook deliveries.
type Publisher struct {
	broker publisher
	now    func() time.Time
}

// NewPublisher returns a publisher that sends through broker.
func NewPublisher(broker publisher) *Publisher {
	return &Publisher{broker: broker, now: time.Now}
}

// Publish enqueues body, which must be valid JSON, and returns once the broker
// confirmed it.
func (p *Publisher) Publish(ctx context.Context, provider, deliveryID string, body []byte) error {
	env, err := json.Marshal(Envelope{
		Provider:   provider,
		DeliveryID: deliveryID,
		ReceivedAt: p.now().UTC(),
		Payload:    body,
	})
	if err != nil {
		return fmt.Errorf("encode envelope: %w", err)
	}
	return p.broker.Publish(ctx, Exchange, "webhook."+provider, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    deliveryID,
		Timestamp:    p.now(),
		Body:         env,
	})
}

type subscriber interface {
	Consume(ctx context.Context, queue string, prefetch int, handle func(amqp.Delivery)) error
}

// EventApplier writes the effect of a provider event.
type EventApplier interface {
	ApplyEvent(ctx context.Context, provider string, ev task.Event) (bool, error)
}

// Consumer turns queued deliveries into tasks.
type Consumer struct {
	broker     subscriber
	registry   *adapter.Registry
	tasks      EventApplier
	log        *slog.Logger
	retryDelay time.Duration

	cancel context.CancelFunc
	done   chan struct{}
}

// NewConsumer returns a consumer that reads from broker, parses deliveries
// with the adapters in registry and applies them with tasks.
func NewConsumer(broker subscriber, registry *adapter.Registry, tasks EventApplier, log *slog.Logger) *Consumer {
	return &Consumer{
		broker:     broker,
		registry:   registry,
		tasks:      tasks,
		log:        log,
		retryDelay: retryDelay,
	}
}

// Start consumes in the background until Stop.
func (c *Consumer) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	go func() {
		defer close(c.done)
		_ = c.broker.Consume(ctx, Queue, prefetch, func(d amqp.Delivery) { c.handle(ctx, d) })
		c.log.Info("consumer stopped")
	}()
	c.log.Info("consumer started", "queue", Queue)
}

// Stop stops taking messages and waits for the one in progress, up to ctx.
func (c *Consumer) Stop(ctx context.Context) error {
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("consumer: stop: %w", ctx.Err())
	}
}

type outcome int

const (
	ack     outcome = iota // processed or deliberately ignored
	reject                 // can never succeed: dead-letter it now
	requeue                // may succeed later: deliver it again
)

// handle settles d. stopping is cancelled by Stop; processing itself runs on
// a separate context so a shutdown does not abort a write halfway.
func (c *Consumer) handle(stopping context.Context, d amqp.Delivery) {
	ctx, cancel := context.WithTimeout(context.Background(), processingTimeout)
	defer cancel()

	var err error
	switch c.process(ctx, d.Body) {
	case ack:
		err = d.Ack(false)
	case reject:
		err = d.Nack(false, false)
	case requeue:
		// A short pause keeps a brief outage from burning every delivery.
		select {
		case <-time.After(c.retryDelay):
		case <-stopping.Done():
		}
		err = d.Nack(false, true)
	}
	if err != nil {
		c.log.Error("could not settle message", "message_id", d.MessageId, "error", err)
	}
}

func (c *Consumer) process(ctx context.Context, body []byte) outcome {
	var env Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		c.log.Error("dead-lettering message: invalid envelope", "error", err)
		return reject
	}
	log := c.log.With("provider", env.Provider, "delivery_id", env.DeliveryID)

	a, ok := c.registry.Lookup(env.Provider)
	if !ok {
		log.Error("dead-lettering message: unknown provider")
		return reject
	}
	ev, err := a.Parse(adapter.Delivery{ID: env.DeliveryID, Body: env.Payload})
	if err != nil {
		log.Error("dead-lettering message: invalid payload", "error", err)
		return reject
	}

	created, err := c.tasks.ApplyEvent(ctx, env.Provider, ev)
	var ve *task.ValidationError
	switch {
	case errors.As(err, &ve):
		log.Error("dead-lettering message: invalid event", "error", err)
		return reject
	case err != nil:
		log.Warn("could not apply event, requeueing", "error", err)
		return requeue
	case ev.Type != task.EventCreated:
		log.Info("event ignored", "type", ev.Type, "external_id", ev.ExternalID)
	case created:
		log.Info("task created", "external_id", ev.ExternalID)
	default:
		log.Info("task already exists, delivery ignored", "external_id", ev.ExternalID)
	}
	return ack
}
