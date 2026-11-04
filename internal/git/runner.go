package git

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"go.uber.org/zap"
)

// realRunner executes system commands
type realRunner struct {
	debug  bool
	logger *zap.Logger
}


// NewRunnerWithLogger creates a new git command runner with logger
func NewRunnerWithLogger(debug bool, logger *zap.Logger) Runner {
	return &realRunner{
		debug:  debug,
		logger: logger,
	}
}

func (r *realRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if r.debug && r.logger != nil {
		r.logger.Debug("Running command",
			zap.String("command", name),
			zap.Strings("args", args))
	}
	output, err := cmd.CombinedOutput()
	if r.debug && r.logger != nil {
		r.logger.Debug("Command output",
			zap.Int("output_length", len(output)),
			zap.Error(err),
			zap.String("output", func() string {
				if len(output) > 0 && len(output) < 1000 {
					return string(output)
				}
				return fmt.Sprintf("<%d bytes>", len(output))
			}()))
	}
	return string(output), err
}

// RunStreaming executes a command, streams its combined output to the terminal
// in real time and returns everything that was printed. Streaming keeps the
// live feedback of long running commands (progress, remote hooks, CI links)
// while still letting callers inspect the output afterwards.
func RunStreaming(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var buf bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)

	err := cmd.Run()
	return buf.String(), err
}
