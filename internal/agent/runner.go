package agent

import "context"

// Runner is the execution backend abstraction.
//
// Implementations must:
//   - Return a channel that is always eventually closed.
//   - Emit EventStarted as the first event.
//   - Emit exactly one terminal event (EventCompleted or EventFailed) before closing.
//   - Respect ctx cancellation; emit EventFailed on cancellation then close.
//   - Be safe to call concurrently.
type Runner interface {
	// Run begins agent execution and returns a channel of events.
	// The channel is closed when execution terminates (success, failure, or cancellation).
	// A non-nil error means execution could not be started; the channel will be nil.
	Run(ctx context.Context, req RunRequest) (<-chan Event, error)

	// Name returns a stable identifier for this runner (e.g. "fake", "claude-code").
	Name() string
}
