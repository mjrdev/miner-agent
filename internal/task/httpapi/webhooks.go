package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/jrgomesdev/miner/internal/task/adapter"
)

// Publisher enqueues an accepted webhook body and returns once the broker
// confirmed it.
type Publisher interface {
	Publish(ctx context.Context, provider, deliveryID string, body []byte) error
}

// Webhooks receives provider webhooks and enqueues them.
type Webhooks struct {
	registry *adapter.Registry
	pub      Publisher
	log      *slog.Logger
}

// NewWebhooks returns the webhook handlers for the providers in registry.
func NewWebhooks(registry *adapter.Registry, pub Publisher, log *slog.Logger) *Webhooks {
	return &Webhooks{registry: registry, pub: pub, log: log}
}

// Register adds the webhook routes to mux.
func (h *Webhooks) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/{provider}", h.receive)
}

type deliveryResponse struct {
	DeliveryID string `json:"delivery_id"`
}

func (h *Webhooks) receive(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	a, ok := h.registry.Lookup(provider)
	if !ok {
		writeError(w, http.StatusNotFound, codeNotFound, "unknown provider")
		return
	}

	body, ok := readBody(w, r)
	if !ok {
		return
	}

	if err := a.Verify(r.Header, body); err != nil {
		if !errors.Is(err, adapter.ErrUnauthorized) {
			h.log.ErrorContext(r.Context(), "webhook verification failed", "provider", provider, "error", err)
		}
		writeError(w, http.StatusUnauthorized, codeUnauthorized, "invalid webhook credentials")
		return
	}

	if len(body) == 0 || !json.Valid(body) {
		writeError(w, http.StatusBadRequest, codeValidationFailed, "body must be valid JSON")
		return
	}

	deliveryID := uuid.Must(uuid.NewV7()).String()
	if err := h.pub.Publish(r.Context(), provider, deliveryID, body); err != nil {
		h.log.ErrorContext(r.Context(), "could not enqueue webhook", "provider", provider, "error", err)
		writeError(w, http.StatusServiceUnavailable, codeUnavailable, "could not enqueue the delivery, retry later")
		return
	}

	h.log.InfoContext(r.Context(), "webhook enqueued", "provider", provider, "delivery_id", deliveryID)
	writeJSON(w, http.StatusAccepted, deliveryResponse{DeliveryID: deliveryID})
}
