package task

// EventType is the kind of change an adapter reports for a ticket.
type EventType string

const (
	EventCreated EventType = "created"
	EventUpdated EventType = "updated"
	EventClosed  EventType = "closed"
)

// Valid reports whether t is a known event type.
func (t EventType) Valid() bool {
	switch t {
	case EventCreated, EventUpdated, EventClosed:
		return true
	}
	return false
}

// Event is a provider ticket change normalized by an adapter. It is the only
// input the ingestion uses to write tasks, whatever the provider.
type Event struct {
	Type        EventType
	ExternalID  string
	URL         *string
	Title       string
	Description string
	// Raw is the original provider payload, kept for auditing and reprocessing.
	Raw []byte
}
