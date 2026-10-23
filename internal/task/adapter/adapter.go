// Package adapter defines the contract every ticket provider implements to
// feed tasks into the service, and the registry that resolves providers by
// name.
package adapter

import (
	"errors"
	"net/http"

	"github.com/jrgomesdev/miner/internal/task"
)

var (
	// ErrUnauthorized means the webhook request could not be authenticated.
	ErrUnauthorized = errors.New("unauthorized webhook request")
	// ErrInvalidPayload means the delivery can never become an event; it is
	// sent to the dead-letter queue without retries.
	ErrInvalidPayload = errors.New("invalid webhook payload")
)

// Delivery is a webhook body accepted by the service.
type Delivery struct {
	// ID identifies the delivery; it is the delivery_id returned with 202.
	ID   string
	Body []byte
}

// Adapter translates one provider's webhooks.
type Adapter interface {
	// Name is the provider in /webhooks/{provider} and in task.Provider.
	Name() string
	// Verify authenticates the request before it is enqueued. It returns an
	// error wrapping ErrUnauthorized when the request must be rejected.
	Verify(h http.Header, body []byte) error
	// Parse converts the delivery into a normalized event. It returns an error
	// wrapping ErrInvalidPayload when the body can never be processed.
	Parse(d Delivery) (task.Event, error)
}

// Registry maps provider names to adapters.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry returns a registry with adapters.
func NewRegistry(adapters ...Adapter) *Registry {
	r := &Registry{adapters: make(map[string]Adapter, len(adapters))}
	for _, a := range adapters {
		r.adapters[a.Name()] = a
	}
	return r
}

// Lookup returns the adapter for provider.
func (r *Registry) Lookup(provider string) (Adapter, bool) {
	a, ok := r.adapters[provider]
	return a, ok
}
