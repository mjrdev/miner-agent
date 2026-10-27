package run_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/broker"
	"github.com/jrgomesdev/miner/internal/execution/run"
	"github.com/jrgomesdev/miner/internal/task/ingest"
)

// TestDeadLettersStaySeparate needs TEST_RABBITMQ_URL (a dedicated vhost: its
// queues are purged).
func TestDeadLettersStaySeparate(t *testing.T) {
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL not set")
	}
	ctx := context.Background()
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	b, err := broker.Connect(ctx, url, broker.Setups(ingest.DeclareTopology, run.DeclareTopology), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ch.Close() }()
	for _, q := range []string{run.Queue, run.DeadLetterQueue, ingest.DeadLetterQueue} {
		if _, err := ch.QueuePurge(q, false); err != nil {
			t.Fatalf("purge %s: %v", q, err)
		}
	}

	id := uuid.New()
	if err := run.NewPublisher(b).Publish(ctx, id); err != nil {
		t.Fatal(err)
	}
	var d amqp.Delivery
	for deadline := time.Now().Add(5 * time.Second); ; {
		msg, ok, err := ch.Get(run.Queue, false)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			d = msg
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run message not delivered to the queue")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if d.MessageId != id.String() {
		t.Fatalf("message id = %q", d.MessageId)
	}
	if err := d.Nack(false, false); err != nil {
		t.Fatal(err)
	}

	for deadline := time.Now().Add(5 * time.Second); ; {
		msg, ok, err := ch.Get(run.DeadLetterQueue, true)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			if msg.MessageId != id.String() {
				t.Errorf("dead letter id = %q", msg.MessageId)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("rejected run message not dead-lettered")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if q, err := ch.QueueDeclarePassive(ingest.DeadLetterQueue, true, false, false, false,
		amqp.Table{amqp.QueueTypeArg: amqp.QueueTypeQuorum}); err != nil || q.Messages != 0 {
		t.Errorf("ingestion DLQ = %d messages, %v", q.Messages, err)
	}
}
