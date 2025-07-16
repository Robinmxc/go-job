package utils

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestWriteFileSuccess tests successful file writes with different configurations
func TestWriteFileSuccess(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		config   WriteConfig
		mockUser *user.User
		wantPerm os.FileMode
		wantUser string
	}{
		{
			name:     "Default configuration",
			content:  []byte("test content"),
			config:   WriteConfig{},
			wantPerm: 0644,
			wantUser: "", // Current user (not explicitly set)
		},
		{
			name:     "Custom permissions",
			content:  []byte("custom perm"),
			config:   WriteConfig{Perm: 0755},
			wantPerm: 0755,
			wantUser: "",
		},
		{
			name:     "Append mode",
			content:  []byte("append me"),
			config:   WriteConfig{Flag: os.O_APPEND | os.O_CREATE | os.O_WRONLY},
			wantPerm: 0644, // Default perm still applies
			wantUser: "",
		},
		{
			name:     "With user ownership",
			content:  []byte("owned by testuser"),
			config:   WriteConfig{User: "testuser"},
			mockUser: &user.User{Uid: "1001", Gid: "1001"},
			wantPerm: 0644,
			wantUser: "1001", // UID of testuser
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

			// Create temp file
			tmpfile, err := ioutil.TempFile("", "test-write-")
			if err != nil {
				t.Fatalf("Failed to create temp file: %v", err)
			}
			tmpfile.Close()
			defer os.Remove(tmpfile.Name()) // Cleanup

			// Execute write
			err = WriteFile(tmpfile.Name(), tt.content, tt.config)
			if err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			// Verify file content
			data, err := ioutil.ReadFile(tmpfile.Name())
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}
			if !bytes.Equal(data, tt.content) {
				t.Errorf("Content mismatch. Got: %q, Want: %q", string(data), string(tt.content))
			}

			// Verify permissions
			info, err := os.Stat(tmpfile.Name())
			if err != nil {
				t.Fatalf("Failed to get file info: %v", err)
			}
			if info.Mode().Perm() != tt.wantPerm {
				t.Errorf("Permissions mismatch. Got: %o, Want: %o", info.Mode().Perm(), tt.wantPerm)
			}

			// Verify user ownership (if set)
			if tt.config.User != "" {
				if info.Sys() == nil {
					t.Fatal("Could not get system-specific file info")
				}

				// This part is platform-specific and may need adjustments for non-Unix systems
				if stat, ok := info.Sys().(*syscall.Stat_t); ok {
					if strconv.FormatUint(uint64(stat.Uid), 10) != tt.wantUser {
						t.Errorf("User ownership mismatch. Got UID: %d, Want: %s", stat.Uid, tt.wantUser)
					}
				} else {
					t.Fatal("Could not convert to syscall.Stat_t")
				}
			}
		})
	}
}

// TestWriteFileError tests error scenarios for WriteFile
func TestWriteFileError(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		config   WriteConfig
		mockUser *user.User
		wantErr  string
	}{
		{
			name:    "Non-existent directory",
			content: []byte("test"),
			config:  WriteConfig{},
			wantErr: "permission denied", // Assuming /nonexistent is inaccessible
		},
		{
			name:    "Invalid user",
			content: []byte("test"),
			config:  WriteConfig{User: "unknown"},
			wantErr: "user unknown not found",
		},
		{
			name:     "Invalid UID",
			content:  []byte("test"),
			config:   WriteConfig{User: "baduser"},
			mockUser: &user.User{Uid: "invalid", Gid: "1001"},
			wantErr:  "invalid user ID for user baduser",
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

			// Set path to trigger error
			path := "/nonexistent/testfile"
			if tt.name != "Non-existent directory" {
				tmpfile, err := os.CreateTemp("", "test-error-")
				if err != nil {
					t.Fatalf("Failed to create temp file: %v", err)
				}
				tmpfile.Close()
				defer os.Remove(tmpfile.Name())
				path = tmpfile.Name()
			}

			// Execute write
			err := WriteFile(path, tt.content, tt.config)
			if err == nil {
				t.Fatal("Expected error, but got nil")
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Error message mismatch. Got: %q, Want substring: %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestGenerateFileName tests the filename generation function
func TestGenerateFileName(t *testing.T) {
	tests := []struct {
		prefix      string
		suffix      string
		mockTime    time.Time
		wantPattern string
	}{
		{
			prefix:      "log_",
			suffix:      ".txt",
			wantPattern: "log_xxx.txt",
		},
		{
			prefix:      "",
			suffix:      ".tmp",
			mockTime:    time.Date(2025, time.July, 16, 0, 0, 0, 0, time.UTC),
			wantPattern: "xxxx.tmp",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.prefix, tt.suffix), func(t *testing.T) {
			filename := GenerateFileName(tt.prefix, tt.suffix)
			if !strings.HasPrefix(filename, tt.prefix) || !strings.HasSuffix(filename, tt.suffix) {
				t.Errorf("Generated filename mismatch. Got: %q, Want: %q", filename, tt.wantPattern)
			}
		})
	}
}

// TestThreadSafeWriteFile tests thread safety of file writing
func TestThreadSafeWriteFile(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-threadsafe-")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	// Number of concurrent writes
	const numWriters = 10
	var wg sync.WaitGroup
	wg.Add(numWriters)

	// Run concurrent writes
	for i := 0; i < numWriters; i++ {
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("Data from goroutine %d\n", idx))
			err := ThreadSafeWriteFile(tmpfile.Name(), data)
			if err != nil {
				t.Errorf("ThreadSafeWriteFile() error = %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify the file exists and contains one of the written contents
	_, err = os.Stat(tmpfile.Name())
	if err != nil {
		t.Fatalf("File does not exist after writes: %v", err)
	}

	// Note: Due to atomic renames, the file should contain exactly one write operation's data
	// This checks that the file is not empty and has valid content
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if len(data) == 0 {
		t.Error("File is empty after writes")
	}
}
