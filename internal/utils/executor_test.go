package utils

import (
	"fmt"
	"os/user"
	"strings"
	"testing"
	"time"
)

// MockUserLooker is a mock implementation of user lookup for testing
type MockUserLooker struct {
	Users map[string]*user.User
	Error error
}

func (m *MockUserLooker) Lookup(username string) (*user.User, error) {
	if m.Error != nil {
		return nil, m.Error
	}

	user, exists := m.Users[username]
	if !exists {
		return nil, fmt.Errorf("user %s not found", username)
	}
	return user, nil
}

func (m *MockUserLooker) LookupId(uid string) (*user.User, error) {
	if m.Error != nil {
		return nil, m.Error
	}

	for _, user := range m.Users {
		if user.Uid == uid {
			return user, nil
		}
	}
	return nil, fmt.Errorf("user with ID %s not found", uid)
}

// TestExecuteCommandSuccess tests successful command execution
func TestExecuteCommandSuccess(t *testing.T) {
	tests := []struct {
		name     string
		config   CommandConfig
		mockUser *user.User
		wantOut  string
	}{
		{
			name: "Simple echo command",
			config: CommandConfig{
				Command: "echo",
				Args:    []string{"hello"},
			},
			wantOut: "hello\n",
		},
		{
			name: "Command with working directory",
			config: CommandConfig{
				Command:    "pwd",
				WorkingDir: "/tmp",
			},
			wantOut: "/tmp\n",
		},
		{
			name: "Command with environment variable",
			config: CommandConfig{
				Command: "sh",
				Args:    []string{"-c", "echo $TEST_VAR"},
				Env:     []string{"TEST_VAR=test_value"},
			},
			wantOut: "test_value\n",
		},
		{
			name: "Command as specific user",
			config: CommandConfig{
				Command: "id",
				Args:    []string{"-un"},
				User:    "root",
			},
			mockUser: &user.User{Uid: "0", Gid: "0"},
			wantOut:  "root\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock user looker
			mockLooker := &MockUserLooker{
				Users: map[string]*user.User{},
			}

			if tt.mockUser != nil {
				mockLooker.Users[tt.config.User] = tt.mockUser
			}

			// Override user lookup for testing
			defaultLooker = mockLooker

			result, err := ExecuteCommand(tt.config)
			if err != nil {
				t.Fatalf("ExecuteCommand() error = %v", err)
			}

			if !result.Successful {
				t.Errorf("Expected command to succeed, but it failed with output: %s", string(result.Output))
			}

			if gotOut := string(result.Output); !strings.Contains(gotOut, tt.wantOut) {
				t.Errorf("Output mismatch. Got: %q, Want substring: %q", gotOut, tt.wantOut)
			}
		})
	}
}

// TestExecuteCommandTimeout tests command timeout scenario
func TestExecuteCommandTimeout(t *testing.T) {
	config := CommandConfig{
		Command: "sleep",
		Args:    []string{"5"},
		Timeout: 1 * time.Second,
	}

	// Override user lookup for testing
	defaultLooker = &MockUserLooker{}

	result, err := ExecuteCommand(config)
	if result == nil || err == nil {
		t.Fatal("Expected timeout error, but got nil")
	}

	if !result.TimedOut {
		t.Errorf("Expected TimedOut to be true, but got false")
	}

	if result.Successful {
		t.Errorf("Expected command to fail due to timeout, but Successful is true")
	}

	expectedErrMsg := "command timed out after 1s"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("Error message mismatch. Got: %q, Want substring: %q", err.Error(), expectedErrMsg)
	}
}

// TestExecuteCommandError tests various error scenarios
func TestExecuteCommandError(t *testing.T) {
	tests := []struct {
		name    string
		config  CommandConfig
		wantErr string
	}{
		{
			name: "Non-existent command",
			config: CommandConfig{
				Command: "/non-existent-command",
			},
			wantErr: "no such file or directory",
		},
		{
			name: "Non-existent working directory",
			config: CommandConfig{
				Command:    "ls",
				WorkingDir: "/non-existent-dir",
			},
			wantErr: "no such file or directory",
		},
		{
			name: "Command failure",
			config: CommandConfig{
				Command: "ls",
				Args:    []string{"/non-existent-file"},
			},
			wantErr: "exit status 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override user lookup for testing
			defaultLooker = &MockUserLooker{}

			result, err := ExecuteCommand(tt.config)
			if err == nil {
				t.Fatalf("Expected error, but got nil")
			}

			if result.Successful {
				t.Errorf("Expected command to fail, but Successful is true")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Error message mismatch. Got: %q, Want substring: %q", err.Error(), tt.wantErr)
			}
		})
	}
}
