// Package exec provides timeout-protected command execution wrappers.
package exec

import (
	"context"
	"os/exec"
	"time"
)

// RunWithTimeout executes a command with a timeout.
// Returns the command output or an error if the command fails or times out.
func RunWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// Run executes a command with a timeout and returns only the error.
// Useful for commands where output is not needed.
func Run(timeout time.Duration, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Run()
}

// RunWithContext executes a command with the provided context.
// The context can be used for cancellation (e.g., when a preview becomes stale).
func RunWithContext(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// Default timeout for most operations (5 seconds).
const DefaultTimeout = 5 * time.Second
