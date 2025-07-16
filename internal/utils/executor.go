package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var defaultLooker userLooker = &defaultUserLooker{}

// CommandConfig holds the configuration options for command execution
type CommandConfig struct {
	Command    string        // The command to execute
	Args       []string      // Command arguments
	User       string        // User to execute the command as (optional)
	WorkingDir string        // Working directory for the command (optional)
	Env        []string      // Environment variables to set (optional)
	Timeout    time.Duration // Command execution timeout (optional)
}

// CommandResult holds the result of command execution
type CommandResult struct {
	Command    string // The executed command
	TimedOut   bool   // Whether the command timed out
	Successful bool   // Whether the command executed successfully
	Output     []byte // Standard output bytes and standard error(combined)
	ExecError  error  // Execution error (if any)
}

// ExecuteCommand executes a command with the provided configuration
func ExecuteCommand(config CommandConfig) (*CommandResult, error) {
	// Create context with timeout
	ctx := context.Background()
	var cancel context.CancelFunc

	if config.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	}

	// Create the command
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)

	// Set working directory if specified
	if config.WorkingDir != "" {
		cmd.Dir = config.WorkingDir
	}

	// Set environment variables if specified
	if len(config.Env) > 0 {
		cmd.Env = append(os.Environ(), config.Env...)
	}

	// Configure user if specified
	if config.User != "" {
		user, err := lookupUser(config.User, defaultLooker)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup user %s: %w", config.User, err)
		}

		uid, err := strconv.Atoi(user.Uid)
		if err != nil {
			return nil, fmt.Errorf("invalid user ID for user %s: %w", config.User, err)
		}

		gid, err := strconv.Atoi(user.Gid)
		if err != nil {
			return nil, fmt.Errorf("invalid group ID for user %s: %w", config.User, err)
		}

		cmd.SysProcAttr = &syscall.SysProcAttr{
			Credential: &syscall.Credential{
				Uid: uint32(uid),
				Gid: uint32(gid),
			},
		}
	}

	// Execute the command
	out, err := cmd.CombinedOutput()

	// Process result
	result := &CommandResult{
		Command:    config.Command + " " + strings.Join(config.Args, " "),
		TimedOut:   false,
		Successful: false,
		Output:     out,
		ExecError:  err,
	}

	// Check for timeout
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		result.ExecError = fmt.Errorf("command timed out after %v", config.Timeout)
		return result, result.ExecError
	}

	result.Successful = err == nil
	return result, err
}
