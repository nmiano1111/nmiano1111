package verifier

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// Shell runs an arbitrary shell command as the verification step.
// A zero exit code means passed; any non-zero exit means failed.
type Shell struct {
	Command string // passed to sh -c
}

// Verify implements Verifier.
func (v *Shell) Verify(ctx context.Context, workDir string) (Result, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", v.Command)
	if workDir != "" {
		cmd.Dir = workDir
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	result := Result{
		Command: v.Command,
		Passed:  err == nil,
		Output:  out.String(),
	}
	if err != nil && ctx.Err() == nil {
		// Execution error (non-zero exit) is captured in Passed=false;
		// we don't propagate it as a Go error.
		return result, nil
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("verifier cancelled: %w", ctx.Err())
	}
	return result, nil
}
