package ingest_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/broker"
	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
	"github.com/jrgomesdev/miner/internal/task/adapter/local"
	"github.com/jrgomesdev/miner/internal/task/ingest"
	"github.com/jrgomesdev/miner/internal/task/postgres"
	"github.com/jrgomesdev/miner/internal/testdb"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

// connectBroker connects to TEST_RABBITMQ_URL (use a dedicated vhost: its
// ingestion queues are purged) and declares the ingestion topology.
func connectBroker(t *testing.T) *broker.Broker {
	t.Helper()
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL not set")
	}
	b, err := broker.Connect(context.Background(), url, ingest.DeclareTopology, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	for _, q := range []string{ingest.Queue, ingest.DeadLetterQueue} {
		if _, err := ch.QueuePurge(q, false); err != nil {
			t.Fatalf("purge %s (does the topology exist?): %v", q, err)
		}
	}
	return b
}

// deadLettered waits for a message on the dead-letter queue and returns its id.
func deadLettered(t *testing.T) string {
	t.Helper()
	conn, err := amqp.Dial(os.Getenv("TEST_RABBITMQ_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		msg, ok, err := ch.Get(ingest.DeadLetterQueue, true)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			return msg.MessageId
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no message reached the dead-letter queue")
	return ""
}

func startConsumer(t *testing.T, b *broker.Broker, tasks ingest.EventApplier) {
	t.Helper()
	c := ingest.NewConsumer(b, adapter.NewRegistry(local.New("secret")), tasks, discardLogger())
	c.Start()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := c.Stop(ctx); err != nil {
			t.Errorf("Stop() = %v", err)
		}
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestIngestionCreatesTasksOnce(t *testing.T) {
	ctx := context.Background()
	svc := task.NewService(postgres.NewRepository(testdb.Pool(t)))
	b := connectBroker(t)
	startConsumer(t, b, svc)
	pub := ingest.NewPublisher(b)

	body := []byte(`{"title":"Corrigir login","description":"Implemente ..."}`)
	for range 2 {
		if err := pub.Publish(ctx, "local", "delivery-1", body); err != nil {
			t.Fatal(err)
		}
	}
	if err := pub.Publish(ctx, "local", "delivery-2", body); err != nil {
		t.Fatal(err)
	}

	var tasks []task.Task
	waitFor(t, "two tasks", func() bool {
		tasks, _, _ = svc.List(ctx, task.ListFilter{Limit: 10})
		return len(tasks) >= 2
	})
	// Give the duplicate time to be processed before checking it was ignored.
	time.Sleep(300 * time.Millisecond)
	tasks, total, _ := svc.List(ctx, task.ListFilter{Limit: 10})
	if total != 2 {
		t.Fatalf("total = %d, want 2 (one per delivery id)", total)
	}
	for _, tk := range tasks {
		if tk.Provider != "local" || tk.Status != task.StatusDraft || tk.Title != "Corrigir login" {
			t.Errorf("unexpected task %+v", tk)
		}
	}
}

func TestInvalidPayloadIsDeadLettered(t *testing.T) {
	b := connectBroker(t)
	var applied atomic.Int32
	startConsumer(t, b, applier(func() error { applied.Add(1); return nil }))

	if err := ingest.NewPublisher(b).Publish(context.Background(), "local", "bad-1", []byte(`{"description":"sem título"}`)); err != nil {
		t.Fatal(err)
	}
	if id := deadLettered(t); id != "bad-1" {
		t.Errorf("dead-lettered %q, want bad-1", id)
	}
	if applied.Load() != 0 {
		t.Error("invalid payload reached the service")
	}
}

func TestTransientErrorsAllowFiveDeliveries(t *testing.T) {
	b := connectBroker(t)
	var attempts atomic.Int32
	startConsumer(t, b, applier(func() error {
		attempts.Add(1)
		return errors.New("database unavailable")
	}))

	if err := ingest.NewPublisher(b).Publish(context.Background(), "local", "retry-1", []byte(`{"title":"T","description":"D"}`)); err != nil {
		t.Fatal(err)
	}
	if id := deadLettered(t); id != "retry-1" {
		t.Errorf("dead-lettered %q, want retry-1", id)
	}
	if got := attempts.Load(); got != 5 {
		t.Errorf("deliveries before dead-lettering = %d, want 5", got)
	}
}

type applier func() error

func (f applier) ApplyEvent(context.Context, string, task.Event) (bool, error) {
	err := f()
	return err == nil, err
}
