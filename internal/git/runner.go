package git

import (
	"context"
	"fmt"
	"os/exec"

	"go.uber.org/zap"
)

// realRunner 实际执行系统命令
type realRunner struct {
	debug  bool
	logger *zap.Logger
}

// NewRunner creates a new git command runner
func NewRunner(debug bool) Runner {
	return &realRunner{
		debug: debug,
		// Logger will be set when available
	}
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
