package broker

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewJSONHandler(io.Discard, nil)) }

func noSetup(*amqp.Channel) error { return nil }

func TestConnectTimesOut(t *testing.T) {
	start := time.Now()
	_, err := connect(context.Background(), "amqp://u:secret@127.0.0.1:1/", noSetup, discardLogger(),
		300*time.Millisecond, 50*time.Millisecond)
	if err == nil {
		t.Fatal("connect() succeeded against an unreachable address")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("connect() took %s, want about the 300ms timeout", elapsed)
	}
}

func TestConnectRejectsInvalidURL(t *testing.T) {
	_, err := Connect(context.Background(), "http://u:secret@host/", noSetup, discardLogger())
	if err == nil || err.Error() != "broker: invalid RABBITMQ_URL" {
		t.Errorf("error = %v, want invalid RABBITMQ_URL without the URL", err)
	}
}

// connectTest connects to TEST_RABBITMQ_URL and declares a temporary queue,
// skipping the test when the variable is not set.
func connectTest(t *testing.T) (*Broker, string) {
	t.Helper()
	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL not set")
	}
	queue := "broker.test." + uuid.NewString()
	setup := func(ch *amqp.Channel) error {
		_, err := ch.QueueDeclare(queue, false, true, false, false, nil)
		return err
	}
	b, err := Connect(context.Background(), url, setup, discardLogger())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b, queue
}

func TestPublishAndConsume(t *testing.T) {
	b, queue := connectTest(t)
	if err := b.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() = %v", err)
	}

	if err := b.Publish(context.Background(), "", queue, amqp.Publishing{Body: []byte("hello")}); err != nil {
		t.Fatalf("Publish() = %v", err)
	}

	got := consumeOne(t, b, queue)
	if string(got) != "hello" {
		t.Errorf("body = %q, want hello", got)
	}
}

func TestReconnect(t *testing.T) {
	b, queue := connectTest(t)

	// Closing the connection behind the broker's back simulates a drop.
	if err := b.current().Close(); err != nil {
		t.Fatal(err)
	}
	if err := b.Ping(context.Background()); err == nil {
		t.Fatal("Ping() = nil right after the connection was lost")
	}

	deadline := time.Now().Add(10 * time.Second)
	for b.Ping(context.Background()) != nil {
		if time.Now().After(deadline) {
			t.Fatal("broker did not reconnect")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// The setup ran again (the auto-delete queue was recreated) and publishing
	// works on a fresh channel.
	if err := b.Publish(context.Background(), "", queue, amqp.Publishing{Body: []byte("again")}); err != nil {
		t.Fatalf("Publish() after reconnect = %v", err)
	}
	if got := consumeOne(t, b, queue); string(got) != "again" {
		t.Errorf("body = %q, want again", got)
	}
}

func TestPublishWhileDisconnected(t *testing.T) {
	b, queue := connectTest(t)
	b.setConn(nil)
	if err := b.Publish(context.Background(), "", queue, amqp.Publishing{Body: []byte("x")}); err == nil {
		t.Error("Publish() succeeded while disconnected")
	}
}

func consumeOne(t *testing.T, b *Broker, queue string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var body []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = b.Consume(ctx, queue, 1, func(d amqp.Delivery) {
			body = d.Body
			_ = d.Ack(false)
			cancel()
		})
	}()
	<-done
	if body == nil {
		t.Fatal("no message consumed")
	}
	return body
}
