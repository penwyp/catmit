package llm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"go.uber.org/zap"
)

// CLIProvider implements LLM API calls via local CLI tools.
// Supports various AI CLI tools like claude, gemini, qwen, aichat, ollama, etc.
type CLIProvider struct {
	toolPath string      // Absolute path to the CLI tool
	toolName string      // Base name of the tool (claude, gemini, etc.)
	logger   *zap.Logger // Logger for debugging
}

// NewCLIProvider creates a new CLI-based LLM Provider.
// Reads CATMIT_LLM_CLI_TOOL from environment variable.
// The tool can be either a binary name (will be searched in PATH) or an absolute path.
// If the tool is not found or not executable, returns an error.
func NewCLIProvider(logger *zap.Logger) *CLIProvider {
	cliTool := os.Getenv("CATMIT_LLM_CLI_TOOL")
	if cliTool == "" {
		// This shouldn't happen as client.go should check CATMIT_LLM_PROVIDER=cli first
		// But we handle it defensively
		panic("CATMIT_LLM_CLI_TOOL environment variable is required when CATMIT_LLM_PROVIDER=cli")
	}

	if logger != nil {
		logger.Debug("Initializing CLI Provider",
			zap.String("cli_tool", cliTool),
			zap.String("provider", "cli"))
	}

	var toolPath string
	var err error

	// Check if it's an absolute path
	if filepath.IsAbs(cliTool) {
		toolPath = cliTool
		if logger != nil {
			logger.Debug("Using absolute path for CLI tool",
				zap.String("path", toolPath))
		}
		// Verify the file exists and is executable
		if _, err := os.Stat(toolPath); err != nil {
			if logger != nil {
				logger.Error("CLI tool not found at specified path",
					zap.String("path", toolPath),
					zap.Error(err))
			}
			panic(fmt.Sprintf("CLI tool not found at specified path: %s", toolPath))
		}
	} else {
		// It's a command name, search in PATH
		if logger != nil {
			logger.Debug("Searching for CLI tool in PATH",
				zap.String("tool", cliTool))
		}
		toolPath, err = exec.LookPath(cliTool)
		if err != nil {
			if logger != nil {
				logger.Error("CLI tool not found in PATH",
					zap.String("tool", cliTool),
					zap.Error(err))
			}
			panic(fmt.Sprintf("CLI tool '%s' not found in PATH: %v", cliTool, err))
		}
		if logger != nil {
			logger.Debug("Found CLI tool in PATH",
				zap.String("tool", cliTool),
				zap.String("path", toolPath))
		}
	}

	// Verify it's executable
	fileInfo, err := os.Stat(toolPath)
	if err != nil {
		if logger != nil {
			logger.Error("Cannot access CLI tool",
				zap.String("path", toolPath),
				zap.Error(err))
		}
		panic(fmt.Sprintf("Cannot access CLI tool: %v", err))
	}
	if fileInfo.Mode()&0111 == 0 {
		if logger != nil {
			logger.Error("CLI tool is not executable",
				zap.String("path", toolPath),
				zap.String("mode", fileInfo.Mode().String()))
		}
		panic(fmt.Sprintf("CLI tool is not executable: %s", toolPath))
	}

	// Extract tool name from path
	toolName := strings.ToLower(filepath.Base(toolPath))
	// Remove common suffixes like .exe
	toolName = strings.TrimSuffix(toolName, ".exe")

	return &CLIProvider{
		toolPath: toolPath,
		toolName: toolName,
		logger:   logger,
	}
}

// GetCompletion implements the LLMProvider interface for CLI tools.
func (p *CLIProvider) GetCompletion(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.logger != nil {
		p.logger.Debug("CLI Provider GetCompletion called",
			zap.String("tool", p.toolName),
			zap.Int("system_prompt_length", len(systemPrompt)),
			zap.Int("user_prompt_length", len(userPrompt)))
	}

	// Combine prompts for CLI tools
	fullPrompt := p.buildFullPrompt(systemPrompt, userPrompt)

	if p.logger != nil {
		p.logger.Debug("Combined prompt",
			zap.String("tool", p.toolName),
			zap.Int("full_prompt_length", len(fullPrompt)),
			zap.String("prompt_preview", truncateForLog(fullPrompt, 100)))
	}

	// Build command based on tool type
	cmd := p.buildCommand(ctx, fullPrompt, false)

	if p.logger != nil {
		p.logger.Debug("Command built",
			zap.String("tool", p.toolName),
			zap.String("command", cmd.Path),
			zap.Strings("args", cmd.Args[1:]),
			zap.Bool("uses_stdin", p.needsStdinInput()))
	}

	// Execute command and capture output
	if p.logger != nil {
		p.logger.Debug("Starting command execution",
			zap.String("tool", p.toolName),
			zap.String("command_string", fmt.Sprintf("%s %s", cmd.Path, strings.Join(cmd.Args[1:], " "))))
	}
	output, err := p.executeCommand(cmd, fullPrompt)
	if err != nil {
		if p.logger != nil {
			p.logger.Error("CLI tool execution failed",
				zap.String("tool", p.toolName),
				zap.Error(err))
		}
		return "", errors.Wrap(errors.ErrTypeLLM, fmt.Sprintf("CLI tool '%s' execution failed", p.toolName), err)
	}

	if p.logger != nil {
		p.logger.Debug("Command executed successfully",
			zap.String("tool", p.toolName),
			zap.Int("output_length", len(output)))
	}

	// Clean output (remove ANSI codes, etc.)
	cleanOutput := p.cleanOutput(output)

	if p.logger != nil {
		p.logger.Debug("Output cleaned",
			zap.String("tool", p.toolName),
			zap.Int("cleaned_length", len(cleanOutput)),
			zap.String("output_preview", truncateForLog(cleanOutput, 100)))
	}

	return cleanOutput, nil
}

// GetCompletionStream implements streaming for CLI tools.
func (p *CLIProvider) GetCompletionStream(ctx context.Context, systemPrompt, userPrompt string) (<-chan string, <-chan error) {
	contentChan := make(chan string, 100)
	errChan := make(chan error, 1)

	if p.logger != nil {
		p.logger.Debug("CLI Provider GetCompletionStream called",
			zap.String("tool", p.toolName),
			zap.Int("system_prompt_length", len(systemPrompt)),
			zap.Int("user_prompt_length", len(userPrompt)))
	}

	go func() {
		defer close(contentChan)
		defer close(errChan)

		// Combine prompts
		fullPrompt := p.buildFullPrompt(systemPrompt, userPrompt)

		// Build command for streaming
		cmd := p.buildCommand(ctx, fullPrompt, true)

		if p.logger != nil {
			p.logger.Debug("Stream command built",
				zap.String("tool", p.toolName),
				zap.String("command", cmd.Path),
				zap.Strings("args", cmd.Args[1:]))
		}

		// Set up pipes
		stdin, err := cmd.StdinPipe()
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create stdin pipe", err)
			return
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create stdout pipe", err)
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			errChan <- errors.Wrap(errors.ErrTypeLLM, "failed to create stderr pipe", err)
			return
		}

		// Start command
		if err := cmd.Start(); err != nil {
			if p.logger != nil {
				p.logger.Error("Failed to start CLI tool",
					zap.String("tool", p.toolName),
					zap.Error(err))
			}
			errChan <- errors.Wrap(errors.ErrTypeLLM, fmt.Sprintf("failed to start CLI tool '%s'", p.toolName), err)
			return
		}

		if p.logger != nil {
			p.logger.Debug("CLI tool started",
				zap.String("tool", p.toolName),
				zap.Int("pid", cmd.Process.Pid))
		}

		// Write prompt to stdin if needed
		if p.needsStdinInput() {
			go func() {
				defer stdin.Close()
				_, _ = io.WriteString(stdin, fullPrompt)
			}()
		}

		// Capture stderr for error messages
		stderrBytes := &bytes.Buffer{}
		go func() {
			_, _ = io.Copy(stderrBytes, stderr)
		}()

		// Read stdout and send to channel
		scanner := bufio.NewScanner(stdout)
		lineCount := 0
		for scanner.Scan() {
			line := scanner.Text()
			lineCount++

			if p.logger != nil && lineCount <= 5 {
				// Log first few lines for debugging
				p.logger.Debug("Stream line received",
					zap.String("tool", p.toolName),
					zap.Int("line_number", lineCount),
					zap.String("raw_line", truncateForLog(line, 100)))
			}

			// Process based on tool type
			cleaned := p.processStreamLine(line)
			if cleaned != "" {
				select {
				case contentChan <- cleaned:
				case <-ctx.Done():
					if p.logger != nil {
						p.logger.Debug("Context cancelled, killing process",
							zap.String("tool", p.toolName),
							zap.Int("pid", cmd.Process.Pid))
					}
					_ = cmd.Process.Kill()
					return
				}
			}
		}

		if p.logger != nil {
			p.logger.Debug("Stream reading completed",
				zap.String("tool", p.toolName),
				zap.Int("total_lines", lineCount))
		}

		// Wait for command to finish
		if err := cmd.Wait(); err != nil {
			// Check if process was killed due to context cancellation
			if ctx.Err() != nil {
				if p.logger != nil {
					p.logger.Debug("Process terminated due to context cancellation",
						zap.String("tool", p.toolName))
				}
				return
			}

			// Include stderr in error message
			stderrStr := stderrBytes.String()
			if p.logger != nil {
				p.logger.Error("CLI tool process failed",
					zap.String("tool", p.toolName),
					zap.Error(err),
					zap.String("stderr", truncateForLog(stderrStr, 500)))
			}
			if stderrStr != "" {
				errChan <- errors.New(errors.ErrTypeLLM, fmt.Sprintf("CLI tool failed: %s\nStderr: %s", err, stderrStr))
			} else {
				errChan <- errors.Wrap(errors.ErrTypeLLM, "CLI tool failed", err)
			}
		} else {
			if p.logger != nil {
				p.logger.Debug("CLI tool process completed successfully",
					zap.String("tool", p.toolName))
			}
		}
	}()

	return contentChan, errChan
}

// buildFullPrompt combines system and user prompts based on tool requirements.
func (p *CLIProvider) buildFullPrompt(systemPrompt, userPrompt string) string {
	// Most CLI tools expect a single prompt
	// We'll format it in a way that preserves the intent
	if systemPrompt != "" && userPrompt != "" {
		return fmt.Sprintf("%s\n\n%s", systemPrompt, userPrompt)
	} else if systemPrompt != "" {
		return systemPrompt
	}
	return userPrompt
}

// buildCommand constructs the appropriate command based on the CLI tool.
func (p *CLIProvider) buildCommand(ctx context.Context, prompt string, streaming bool) *exec.Cmd {
	var cmd *exec.Cmd

	switch p.toolName {
	case "claude":
		// Claude Code CLI
		args := []string{"-p", prompt}
		if streaming {
			args = append(args, "--output-format", "stream-json")
		}
		cmd = exec.CommandContext(ctx, p.toolPath, args...)

	case "cursor-agent", "cursor":
		// Cursor Agent CLI
		args := []string{"-p", prompt}
		if !streaming {
			// For non-streaming, use json format (single line output)
			args = append(args, "--output-format", "json")
		} else {
			// For streaming, use stream-json format
			args = append(args, "--output-format", "stream-json")
		}
		cmd = exec.CommandContext(ctx, p.toolPath, args...)

	case "gemini":
		// Gemini CLI - pipeline mode
		cmd = exec.CommandContext(ctx, p.toolPath, "-p", prompt)

	case "qwen", "qwen-code":
		// Qwen Code (based on Gemini CLI)
		cmd = exec.CommandContext(ctx, p.toolPath, "-p", prompt)

	case "aichat":
		// AIChat - non-streaming mode for reliability
		cmd = exec.CommandContext(ctx, p.toolPath, "-S")
		// AIChat reads from stdin

	case "ollama":
		// Ollama run with a model (user should specify model in another env var if needed)
		model := os.Getenv("CATMIT_LLM_MODEL")
		if model == "" {
			model = "llama2" // Default model
		}
		cmd = exec.CommandContext(ctx, p.toolPath, "run", model, prompt)

	default:
		// Generic CLI tool - assume it accepts prompt as argument
		// This is a best-effort approach for unknown tools
		cmd = exec.CommandContext(ctx, p.toolPath)
		// Will use stdin for prompt
	}

	return cmd
}

// needsStdinInput returns true if the tool expects input via stdin.
func (p *CLIProvider) needsStdinInput() bool {
	switch p.toolName {
	case "aichat":
		return true
	case "claude", "gemini", "qwen", "qwen-code", "ollama", "cursor-agent", "cursor":
		return false
	default:
		// For unknown tools, we'll try stdin
		return true
	}
}

// executeCommand runs the command and returns the output.
func (p *CLIProvider) executeCommand(cmd *exec.Cmd, prompt string) (string, error) {
	// Set up stdin if needed
	if p.needsStdinInput() {
		cmd.Stdin = strings.NewReader(prompt)
		if p.logger != nil {
			p.logger.Debug("Setting stdin for command",
				zap.String("tool", p.toolName),
				zap.Int("prompt_length", len(prompt)))
		}
	}

	// Capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if p.logger != nil {
		p.logger.Debug("Executing command",
			zap.String("tool", p.toolName),
			zap.String("path", cmd.Path),
			zap.Strings("args", cmd.Args[1:]))
	}

	// Run command
	err := cmd.Run()

	// Check for errors
	if err != nil {
		stderrStr := stderr.String()
		if p.logger != nil {
			p.logger.Error("Command execution failed",
				zap.String("tool", p.toolName),
				zap.Error(err),
				zap.String("stderr", truncateForLog(stderrStr, 500)),
				zap.String("stdout", truncateForLog(stdout.String(), 200)))
		}
		if stderrStr != "" {
			return "", fmt.Errorf("%w\nStderr: %s", err, stderrStr)
		}
		return "", err
	}

	if p.logger != nil {
		p.logger.Debug("Command executed successfully",
			zap.String("tool", p.toolName),
			zap.Int("stdout_size", stdout.Len()),
			zap.Int("stderr_size", stderr.Len()))
	}

	// Return stdout
	return stdout.String(), nil
}

// cleanOutput removes ANSI escape codes and other unwanted characters.
func (p *CLIProvider) cleanOutput(output string) string {
	// Special handling for cursor-agent JSON output
	if p.toolName == "cursor-agent" || p.toolName == "cursor" {
		// Check if it's JSON format (non-streaming)
		if strings.HasPrefix(strings.TrimSpace(output), `{"type":"result"`) {
			// Parse the result field from JSON
			if strings.Contains(output, `"result":"`) {
				start := strings.Index(output, `"result":"`)
				if start != -1 {
					start += 10 // Skip past "result":"
					end := strings.Index(output[start:], `"`)
					if end > 0 {
						text := output[start : start+end]
						// Unescape JSON
						text = strings.ReplaceAll(text, `\n`, "\n")
						text = strings.ReplaceAll(text, `\"`, `"`)
						text = strings.ReplaceAll(text, `\\`, `\`)
						// Remove markdown annotations if present
						lines := strings.Split(text, "\n")
						if len(lines) > 0 {
							// Take the first line as the main result
							result := lines[0]
							// If there are additional lines with markdown, skip them
							if len(lines) > 1 && strings.HasPrefix(lines[1], "-") {
								// Just use the first line
								return strings.TrimSpace(result)
							}
							return strings.TrimSpace(text)
						}
					}
				}
			}
		}
	}

	// Remove ANSI escape sequences
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	output = ansiRegex.ReplaceAllString(output, "")

	// Remove other control characters except newlines and tabs
	controlRegex := regexp.MustCompile(`[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`)
	output = controlRegex.ReplaceAllString(output, "")

	// Trim excessive whitespace
	output = strings.TrimSpace(output)

	return output
}

// processStreamLine processes a single line from streaming output.
func (p *CLIProvider) processStreamLine(line string) string {
	// Tool-specific processing
	switch p.toolName {
	case "claude":
		// Claude with --output-format stream-json returns JSON lines
		// We need to parse and extract content
		// For simplicity, we'll look for content in the JSON
		if strings.Contains(line, `"content":`) {
			// Basic extraction (proper JSON parsing would be better)
			start := strings.Index(line, `"content":"`) + 11
			end := strings.Index(line[start:], `"`)
			if end > 0 {
				return line[start : start+end]
			}
		}
		return ""

	case "cursor-agent", "cursor":
		// Cursor Agent outputs JSON lines with different message types
		// We need to extract content from assistant messages
		if strings.Contains(line, `"type":"assistant"`) && strings.Contains(line, `"text":"`) {
			// Extract text content from assistant messages
			// Looking for pattern: "text":"..."
			start := strings.Index(line, `"text":"`)
			if start != -1 {
				start += 8 // Skip past "text":"
				end := strings.Index(line[start:], `"`)
				if end > 0 {
					text := line[start : start+end]
					// Unescape basic JSON escapes
					text = strings.ReplaceAll(text, `\n`, "\n")
					text = strings.ReplaceAll(text, `\"`, `"`)
					text = strings.ReplaceAll(text, `\\`, `\`)
					return text
				}
			}
		} else if strings.Contains(line, `"type":"result"`) && strings.Contains(line, `"result":"`) {
			// Final result line contains the complete response
			start := strings.Index(line, `"result":"`)
			if start != -1 {
				start += 10 // Skip past "result":"
				end := strings.Index(line[start:], `"`)
				if end > 0 {
					text := line[start : start+end]
					// Unescape JSON
					text = strings.ReplaceAll(text, `\n`, "\n")
					text = strings.ReplaceAll(text, `\"`, `"`)
					text = strings.ReplaceAll(text, `\\`, `\`)
					// For result type, we'll return the full content
					// Remove markdown formatting if present
					text = strings.TrimPrefix(text, "- **result**: ")
					return text
				}
			}
		}
		return ""

	case "ollama":
		// Ollama includes loading indicators that we need to filter
		if strings.Contains(line, ">>>") || strings.Contains(line, "...") {
			return ""
		}
		return p.cleanOutput(line)

	default:
		// For most tools, just clean the output
		return p.cleanOutput(line)
	}
}
