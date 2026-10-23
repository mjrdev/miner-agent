// Package local implements the "local" provider: tasks created without a
// ticket in an external provider, local tests and sources without a dedicated
// adapter. Its payload is already in the canonical format.
package local

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
)

// TokenHeader carries the shared token that authenticates local webhooks.
const TokenHeader = "X-Webhook-Token"

// Adapter authenticates local webhooks with a shared token.
type Adapter struct {
	token string
}

// New returns the local adapter. An empty token rejects every request.
func New(token string) *Adapter {
	return &Adapter{token: token}
}

var _ adapter.Adapter = (*Adapter)(nil)

func (a *Adapter) Name() string { return task.ProviderLocal }

func (a *Adapter) Verify(h http.Header, _ []byte) error {
	if a.token == "" {
		return fmt.Errorf("%w: LOCAL_WEBHOOK_TOKEN is not configured", adapter.ErrUnauthorized)
	}
	if subtle.ConstantTimeCompare([]byte(h.Get(TokenHeader)), []byte(a.token)) != 1 {
		return fmt.Errorf("%w: invalid %s", adapter.ErrUnauthorized, TokenHeader)
	}
	return nil
}

type payload struct {
	Type        task.EventType `json:"type"`
	ExternalID  string         `json:"external_id"`
	URL         *string        `json:"url"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
}

func (a *Adapter) Parse(d adapter.Delivery) (task.Event, error) {
	dec := json.NewDecoder(bytes.NewReader(d.Body))
	dec.DisallowUnknownFields()
	var p payload
	if err := dec.Decode(&p); err != nil {
		return task.Event{}, fmt.Errorf("%w: %v", adapter.ErrInvalidPayload, err)
	}

	if p.Type == "" {
		p.Type = task.EventCreated
	}
	if !p.Type.Valid() {
		return task.Event{}, fmt.Errorf("%w: unknown type %q", adapter.ErrInvalidPayload, p.Type)
	}
	if p.ExternalID == "" {
		// The delivery id is stable across redeliveries, so it keeps them
		// from creating duplicates.
		p.ExternalID = d.ID
	}

	ev := task.Event{
		Type:        p.Type,
		ExternalID:  p.ExternalID,
		URL:         p.URL,
		Title:       p.Title,
		Description: p.Description,
		Raw:         d.Body,
	}
	if p.Type != task.EventCreated {
		return ev, nil
	}
	if _, err := task.NormalizeTitle(p.Title); err != nil {
		return task.Event{}, fmt.Errorf("%w: %v", adapter.ErrInvalidPayload, err)
	}
	if _, err := task.NormalizeDescription(p.Description); err != nil {
		return task.Event{}, fmt.Errorf("%w: %v", adapter.ErrInvalidPayload, err)
	}
	return ev, nil
}
