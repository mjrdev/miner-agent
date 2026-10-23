// Package ingest moves webhook deliveries through RabbitMQ: the publisher
// enqueues them and the consumer turns them into tasks.
package ingest

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	// Exchange receives every webhook delivery, routed by webhook.<provider>.
	Exchange = "tasks.webhooks"
	// Queue holds the deliveries waiting to become tasks.
	Queue = "tasks.ingest"
	// DeadLetterExchange receives the deliveries that could not be processed.
	DeadLetterExchange = "tasks.dlx"
	// DeadLetterQueue keeps the failed deliveries for inspection.
	DeadLetterQueue = "tasks.ingest.dlq"

	// deliveryLimit is how many times a message may be returned to the queue;
	// together with the first delivery it allows 5 deliveries in total.
	deliveryLimit = 4
)

// DeclareTopology declares the exchanges, queues and bindings of the
// ingestion. It is idempotent and runs on every broker (re)connection.
func DeclareTopology(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, amqp.ExchangeTopic, true, false, false, false, nil); err != nil {
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
	return ch.QueueBind(Queue, "webhook.*", Exchange, false, nil)
}
