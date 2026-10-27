package run

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// Exchange receives the runs ready to start.
	Exchange = "tasks.runs"
	// Queue holds the runs waiting for a worker.
	Queue = "tasks.runs"
	// DeadLetterExchange receives the run messages that could not be handled.
	DeadLetterExchange = "tasks.runs.dlx"
	// DeadLetterQueue keeps those messages for inspection; their runs stay
	// queued until cancelled.
	DeadLetterQueue = "tasks.runs.dlq"

	routingKey = "run"
	// deliveryLimit is how many times a message may be returned to the queue.
	deliveryLimit = 4
)

// DeclareTopology declares the exchanges, queues and bindings of the runs.
// It is idempotent and runs on every broker (re)connection.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, amqp.ExchangeDirect, true, false, false, false, nil); err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(DeadLetterExchange, amqp.ExchangeFanout, true, false, false, false, nil); err != nil {
		return err
	}
	quorum := amqp.Table{amqp.QueueTypeArg: amqp.QueueTypeQuorum}
	if _, err := ch.QueueDeclare(DeadLetterQueue, true, false, false, false, quorum); err != nil {
		return err
	}
	if err := ch.QueueBind(DeadLetterQueue, "", DeadLetterExchange, false, nil); err != nil {
		return err
	}
	_, err := ch.QueueDeclare(Queue, true, false, false, false, amqp.Table{
		amqp.QueueTypeArg:        amqp.QueueTypeQuorum,
		"x-delivery-limit":       deliveryLimit,
		"x-dead-letter-exchange": DeadLetterExchange,
	})
	if err != nil {
		return err
	}
	return ch.QueueBind(Queue, routingKey, Exchange, false, nil)
}

// Message is what the queue carries: only the run id, since the database
// holds the run.
type Message struct {
	RunID uuid.UUID `json:"run_id"`
}

type publisher interface {
	Publish(ctx context.Context, exchange, key string, msg amqp.Publishing) error
}

// Publisher enqueues runs.
type Publisher struct {
	broker publisher
}

// NewPublisher returns a publisher that sends through broker.
func NewPublisher(broker publisher) *Publisher {
	return &Publisher{broker: broker}
}

// Publish enqueues the run and returns once the broker confirmed it.
func (p *Publisher) Publish(ctx context.Context, runID uuid.UUID) error {
	body, err := json.Marshal(Message{RunID: runID})
	if err != nil {
		return fmt.Errorf("encode run message: %w", err)
	}
	return p.broker.Publish(ctx, Exchange, routingKey, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    runID.String(),
		Body:         body,
	})
}
