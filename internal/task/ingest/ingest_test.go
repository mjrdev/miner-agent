package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
	"github.com/jrgomesdev/miner/internal/task/adapter/local"
)

// acknowledger records how a delivery was settled.
type acknowledger struct {
	acked, nacked, requeued bool
}

func (a *acknowledger) Ack(uint64, bool) error { a.acked = true; return nil }
func (a *acknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.nacked, a.requeued = true, requeue
	return nil
}
func (a *acknowledger) Reject(_ uint64, requeue bool) error { return a.Nack(0, false, requeue) }

type applierFunc func(ctx context.Context, provider string, ev task.Event) (bool, error)

func (f applierFunc) ApplyEvent(ctx context.Context, provider string, ev task.Event) (bool, error) {
	return f(ctx, provider, ev)
}

func newTestConsumer(apply applierFunc) *Consumer {
	c := NewConsumer(nil, adapter.NewRegistry(local.New("secret")), apply, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	c.retryDelay = time.Millisecond
	return c
}

func delivery(t *testing.T, provider, payload string) (amqp.Delivery, *acknowledger) {
	t.Helper()
	body, err := json.Marshal(Envelope{Provider: provider, DeliveryID: "d-1", Payload: json.RawMessage(payload)})
	if err != nil {
		t.Fatal(err)
	}
	ack := &acknowledger{}
	return amqp.Delivery{Acknowledger: ack, Body: body}, ack
}

func TestConsumerSettlesMessages(t *testing.T) {
	transient := errors.New("connection refused")
	tests := []struct {
		name     string
		provider string
		payload  string
		applyErr error
		want     acknowledger
	}{
		{"valid created event", "local", `{"title":"T","description":"D"}`, nil, acknowledger{acked: true}},
		{"ignored updated event", "local", `{"type":"updated","external_id":"x"}`, nil, acknowledger{acked: true}},
		{"invalid payload", "local", `{"description":"sem título"}`, nil, acknowledger{nacked: true}},
		{"unknown provider", "jira", `{}`, nil, acknowledger{nacked: true}},
		{"transient error", "local", `{"title":"T","description":"D"}`, transient, acknowledger{nacked: true, requeued: true}},
		{
			"validation error", "local", `{"title":"T","description":"D"}`,
			&task.ValidationError{Field: "title", Msg: "x"}, acknowledger{nacked: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var applied *task.Event
			c := newTestConsumer(func(_ context.Context, provider string, ev task.Event) (bool, error) {
				if provider != tt.provider {
					t.Errorf("provider = %q, want %q", provider, tt.provider)
				}
				applied = &ev
				return tt.applyErr == nil, tt.applyErr
			})
			d, ack := delivery(t, tt.provider, tt.payload)

			c.handle(context.Background(), d)

			if *ack != tt.want {
				t.Errorf("settled %+v, want %+v", *ack, tt.want)
			}
			if tt.want.acked && applied == nil {
				t.Error("event not applied")
			}
		})
	}
}

func TestConsumerKeepsDeliveryIDAsExternalID(t *testing.T) {
	var got task.Event
	c := newTestConsumer(func(_ context.Context, _ string, ev task.Event) (bool, error) {
		got = ev
		return true, nil
	})
	d, _ := delivery(t, "local", `{"title":"T","description":"D"}`)
	c.handle(context.Background(), d)
	if got.ExternalID != "d-1" {
		t.Errorf("ExternalID = %q, want the delivery id", got.ExternalID)
	}
}

func TestConsumerInvalidEnvelope(t *testing.T) {
	c := newTestConsumer(nil)
	ack := &acknowledger{}
	c.handle(context.Background(), amqp.Delivery{Acknowledger: ack, Body: []byte("not json")})
	if !ack.nacked || ack.requeued {
		t.Errorf("settled %+v, want dead-lettered", *ack)
	}
}

type publisherFunc func(ctx context.Context, exchange, key string, msg amqp.Publishing) error

func (f publisherFunc) Publish(ctx context.Context, exchange, key string, msg amqp.Publishing) error {
	return f(ctx, exchange, key, msg)
}

func TestPublisher(t *testing.T) {
	var gotKey string
	var gotMsg amqp.Publishing
	p := NewPublisher(publisherFunc(func(_ context.Context, exchange, key string, msg amqp.Publishing) error {
		if exchange != Exchange {
			t.Errorf("exchange = %q", exchange)
		}
		gotKey, gotMsg = key, msg
		return nil
	}))

	if err := p.Publish(context.Background(), "local", "d-1", []byte(`{"title":"T"}`)); err != nil {
		t.Fatal(err)
	}
	if gotKey != "webhook.local" || gotMsg.MessageId != "d-1" || gotMsg.DeliveryMode != amqp.Persistent {
		t.Errorf("published key=%q msg=%+v", gotKey, gotMsg)
	}
	var env Envelope
	if err := json.Unmarshal(gotMsg.Body, &env); err != nil {
		t.Fatal(err)
	}
	if env.Provider != "local" || env.DeliveryID != "d-1" || string(env.Payload) != `{"title":"T"}` {
		t.Errorf("envelope = %+v", env)
	}
}
