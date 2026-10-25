package agent

import (
	"encoding/json"
	"fmt"
	"time"
)

// MakeEvent constructs an Event with the given kind and payload.
// The payload is JSON-encoded into Data. Panics on marshalling failure
// (only happens with non-serialisable types, which should never reach here).
func MakeEvent(runID string, seq int, kind EventKind, payload any) Event {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			panic(fmt.Sprintf("agent.MakeEvent: marshal %T: %v", payload, err))
		}
		raw = b
	}
	return Event{
		ID:        fmt.Sprintf("%s-e%04d", runID, seq),
		RunID:     runID,
		Kind:      kind,
		Timestamp: time.Now(),
		Seq:       seq,
		Data:      raw,
	}
}

// DecodeData unmarshals the event's Data field into dst.
func (e Event) DecodeData(dst any) error {
	return json.Unmarshal(e.Data, dst)
}
