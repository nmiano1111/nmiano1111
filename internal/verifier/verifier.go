// Package verifier defines the post-run validation interface and built-in
// implementations. A Verifier runs after an agent completes and checks whether
// its output meets the required criteria.
package verifier

import "context"

// Result is the outcome of a verification run.
type Result struct {
	// Command is the command or description of what was verified.
	Command string
	// Passed is true if verification succeeded.
	Passed bool
	// Output is the combined stdout+stderr of the verification command.
	Output string
}

// Verifier validates agent output after a run completes.
type Verifier interface {
	// Verify runs the check in workDir and returns the result.
	// An error return means the verifier itself failed to run (not that
	// verification failed); in that case Result.Passed should be false.
	Verify(ctx context.Context, workDir string) (Result, error)
}
