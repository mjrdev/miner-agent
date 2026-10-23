package local

import (
	"errors"
	"net/http"
	"testing"

	"github.com/jrgomesdev/miner/internal/task"
	"github.com/jrgomesdev/miner/internal/task/adapter"
)

func TestVerify(t *testing.T) {
	header := func(token string) http.Header {
		h := http.Header{}
		if token != "" {
			h.Set(TokenHeader, token)
		}
		return h
	}
	tests := []struct {
		name, configured, sent string
		ok                     bool
	}{
		{"correct token", "secret", "secret", true},
		{"wrong token", "secret", "nope", false},
		{"missing token", "secret", "", false},
		{"token not configured", "", "", false},
		{"token not configured, any sent", "", "anything", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.configured).Verify(header(tt.sent), nil)
			if tt.ok && err != nil {
				t.Errorf("Verify() = %v, want nil", err)
			}
			if !tt.ok && !errors.Is(err, adapter.ErrUnauthorized) {
				t.Errorf("Verify() = %v, want ErrUnauthorized", err)
			}
		})
	}
}

func TestParse(t *testing.T) {
	a := New("secret")

	ev, err := a.Parse(adapter.Delivery{
		ID:   "delivery-1",
		Body: []byte(`{"title":"Corrigir login","description":"Implemente ..."}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != task.EventCreated || ev.ExternalID != "delivery-1" || ev.Title != "Corrigir login" {
		t.Errorf("unexpected event: %+v", ev)
	}
	if string(ev.Raw) == "" {
		t.Error("raw payload not kept")
	}

	ev, err = a.Parse(adapter.Delivery{
		ID:   "delivery-2",
		Body: []byte(`{"type":"updated","external_id":"abc","url":"https://x/abc"}`),
	})
	if err != nil || ev.Type != task.EventUpdated || ev.ExternalID != "abc" || *ev.URL != "https://x/abc" {
		t.Errorf("updated event = %+v, %v", ev, err)
	}

	invalid := map[string]string{
		"missing title": `{"description":"sem título"}`,
		"blank desc":    `{"title":"T","description":"  "}`,
		"unknown field": `{"title":"T","description":"D","extra":1}`,
		"unknown type":  `{"type":"deleted","title":"T","description":"D"}`,
		"not an object": `["x"]`,
	}
	for name, body := range invalid {
		if _, err := a.Parse(adapter.Delivery{ID: "d", Body: []byte(body)}); !errors.Is(err, adapter.ErrInvalidPayload) {
			t.Errorf("%s: error = %v, want ErrInvalidPayload", name, err)
		}
	}
}
