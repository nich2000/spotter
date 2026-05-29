package script

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"
)

type Runner struct {
	Timeout time.Duration
}

func (r Runner) Run(ctx context.Context, scriptPath string, args ...string) ([]byte, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmdArgs := append([]string{scriptPath}, args...)
	cmd := exec.CommandContext(ctx, "osascript", cmdArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("osascript timeout after %s", timeout)
	}
	if err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("osascript failed: %s", msg)
	}
	return out, nil
}
