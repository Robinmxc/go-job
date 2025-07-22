package utils

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestReadFileOrDir tests the ReadFileOrDir function for files and directories
func TestReadFileOrDir(t *testing.T) {
	// Create temporary test directory
	testDir, err := os.MkdirTemp("", "read-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(testDir) // Cleanup

	// Create test files and directories
	testFile := filepath.Join(testDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create subdirectory (won't be read recursively)
	subDir := filepath.Join(testDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create file inside subdirectory (should not appear in top-level dir read)
	subFile := filepath.Join(subDir, "subfile.txt")
	if err := os.WriteFile(subFile, []byte("hidden content"), 0644); err != nil {
		t.Fatalf("Failed to create subfile: %v", err)
	}

	// Create another top-level file
	anotherFile := filepath.Join(testDir, "another.log")
	if err := os.WriteFile(anotherFile, []byte("log data"), 0644); err != nil {
		t.Fatalf("Failed to create another file: %v", err)
	}

	tests := []struct {
		name              string
		path              string
		wantIsDir         bool
		wantErr           bool
		wantChildrenCount int    // For directories only
		wantContent       string // For files only
	}{
		{
			name:        "Read regular file",
			path:        testFile,
			wantIsDir:   false,
			wantErr:     false,
			wantContent: "hello world",
		},
		{
			name:              "Read directory (non-recursive)",
			path:              testDir,
			wantIsDir:         true,
			wantErr:           false,
			wantChildrenCount: 3, // test.txt + subdir + another.log
		},
		{
			name:        "Read another regular file",
			path:        anotherFile,
			wantIsDir:   false,
			wantErr:     false,
			wantContent: "log data",
		},
		{
			name:      "Read non-existent path",
			path:      filepath.Join(testDir, "nonexistent"),
			wantIsDir: false,
			wantErr:   true,
		},
		{
			name:              "Read subdirectory directly",
			path:              subDir,
			wantIsDir:         true,
			wantErr:           false,
			wantChildrenCount: 1, // Only subfile.txt
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReadFileOrDir(tt.path)

			// Verify error state
			if (result.Error != nil) != tt.wantErr {
				t.Errorf("Error mismatch: got %v, wantErr %v", result.Error, tt.wantErr)
				return
			}

			// Verify directory flag
			if result.IsDir != tt.wantIsDir {
				t.Errorf("IsDir mismatch: got %v, want %v", result.IsDir, tt.wantIsDir)
			}

			// Verify file content for files
			if !tt.wantIsDir && !tt.wantErr {
				if string(result.Content) != tt.wantContent {
					t.Errorf("Content mismatch: got %q, want %q", string(result.Content), tt.wantContent)
				}
				// Files should have no children
				if len(result.Children) != 0 {
					t.Errorf("Files should have no children: got %d children", len(result.Children))
				}
			}

			// Verify children count for directories
			if tt.wantIsDir && !tt.wantErr {
				if len(result.Children) != tt.wantChildrenCount {
					t.Errorf("Children count mismatch: got %d, want %d", len(result.Children), tt.wantChildrenCount)
				}

				// Verify all children have correct paths and types
				childNames := make(map[string]bool)
				for _, child := range result.Children {
					childName := filepath.Base(child.Path)
					childNames[childName] = true

					// Check if child is a directory when expected
					switch childName {
					case "subdir":
						if !child.IsDir {
							t.Errorf("Child %q should be a directory", childName)
						}
					case "test.txt", "another.log", "subfile.txt":
						if child.IsDir {
							t.Errorf("Child %q should be a file", childName)
						}
					}
				}

				// Verify all expected children exist
				switch tt.path {
				case testDir:
					expected := map[string]bool{"test.txt": true, "subdir": true, "another.log": true}
					for name := range expected {
						if !childNames[name] {
							t.Errorf("Missing expected child: %q", name)
						}
					}
				case subDir:
					if !childNames["subfile.txt"] {
						t.Errorf("Missing expected child in subdir: subfile.txt")
					}
				}
			}
		})
	}
}

// TestWriteFileSuccess tests successful file writes with different configurations
func TestWriteFileSuccess(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		config   []WriteConfig
		mockUser *user.User
		wantPerm os.FileMode
		wantUser string
		runtime  string
	}{
		{
			name:     "Default configuration",
			content:  []byte("test content"),
			config:   nil,
			wantPerm: 0644,
			wantUser: "",
		},
		{
			name:    "Custom permissions",
			content: []byte("custom perm"),
			config: []WriteConfig{WriteConfig{
				Perm: 0755,
				Flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
			}},
			wantPerm: 0755,
			wantUser: "",
		},
		{
			name:    "Append mode",
			content: []byte("append me"),
			config: []WriteConfig{WriteConfig{
				Flag: os.O_APPEND | os.O_CREATE | os.O_WRONLY,
				Perm: 0644, // Explicit permissions
			}},
			wantPerm: 0644,
			wantUser: "",
		},
		{
			name:    "With user ownership",
			content: []byte("owned by testuser"),
			config: []WriteConfig{WriteConfig{
				Perm: 0644,
				Flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
				User: "testuser",
			}},
			mockUser: &user.User{Uid: "1001", Gid: "1001"},
			wantPerm: 0644,
			wantUser: "1001",
			runtime:  "linux",
		},
	}

	for _, tt := range tests {
		if len(tt.runtime) > 0 && runtime.GOOS != tt.runtime {
			t.Skipf("Skipping test on %s platform, expect: %s", runtime.GOOS, tt.runtime)
		}
		t.Run(tt.name, func(t *testing.T) {
			// Override user lookup
			mockLooker := &MockUserLooker{Users: make(map[string]*user.User)}
			if tt.mockUser != nil {
				mockLooker.Users[tt.config[0].User] = tt.mockUser
			}
			defaultLooker = mockLooker

			// Generate unique path without creating file
			tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("test-write-%d", time.Now().UnixNano()))
			defer os.Remove(tmpPath)

			// Execute write
			err := WriteFile(tmpPath, tt.content, tt.config...)
			if err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			// Verify content
			data, err := os.ReadFile(tmpPath)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if !bytes.Equal(data, tt.content) {
				t.Errorf("Content mismatch: got %q, want %q", string(data), string(tt.content))
			}

			// Verify permissions
			info, err := os.Stat(tmpPath)
			if err != nil {
				t.Fatalf("Stat() error = %v", err)
			}
			if info.Mode().Perm() != tt.wantPerm {
				t.Errorf("Permissions mismatch: got %o, want %o", info.Mode().Perm(), tt.wantPerm)
			}

			// Verify ownership
			if len(tt.config) > 0 && tt.config[0].User != "" {
				if info.Sys() == nil {
					t.Fatal("Could not get system-specific info")
				}
				stat, ok := info.Sys().(*syscall.Stat_t)
				if !ok {
					t.Fatal("Could not convert to syscall.Stat_t")
				}
				if strconv.FormatUint(uint64(stat.Uid), 10) != tt.wantUser {
					t.Errorf("UID mismatch: got %d, want %s", stat.Uid, tt.wantUser)
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
			wantErr: "no such file or directory",
		},
		{
			name:    "Invalid user",
			content: []byte("test"),
			config: WriteConfig{
				Perm: 0644,
				Flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
				User: "unknown"},
			wantErr: "user unknown not found",
		},
		{
			name:    "Invalid UID",
			content: []byte("test"),
			config: WriteConfig{
				Perm: 0644,
				Flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC,
				User: "baduser"},
			mockUser: &user.User{Uid: "invalid", Gid: "1001"},
			wantErr:  "invalid user ID for user baduser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override user lookup
			mockLooker := &MockUserLooker{Users: make(map[string]*user.User)}
			if tt.mockUser != nil {
				mockLooker.Users[tt.config.User] = tt.mockUser
			}
			defaultLooker = mockLooker

			if tt.name == "Non-existent directory" {
				if runtime.GOOS != "linux" {
					t.Skipf("Skipping test on %s platform, expect linux", runtime.GOOS)
				}
				err := WriteFile("/nonexistent/testfile", tt.content, tt.config)
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Error mismatch: got %v, want %q", err, tt.wantErr)
				}
			} else {
				tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("test-error-%d", time.Now().UnixNano()))
				defer os.Remove(tmpPath)

				err := WriteFile(tmpPath, tt.content, tt.config)
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Error mismatch: got %v, want %q", err, tt.wantErr)
				}
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
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("test-threadsafe-%d", time.Now().UnixNano()))
	defer os.Remove(tmpPath)

	const numWriters = 10
	var wg sync.WaitGroup
	wg.Add(numWriters)

	// Run concurrent writes
	for i := 0; i < numWriters; i++ {
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("Data from goroutine %d\n", idx))
			err := ThreadSafeWriteFile(tmpPath, data)
			if err != nil {
				t.Errorf("ThreadSafeWriteFile() error = %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify file exists and is not empty
	info, err := os.Stat(tmpPath)
	if err != nil {
		t.Fatalf("File does not exist: %v", err)
	}
	if info.Size() == 0 {
		t.Error("File is empty after writes")
	}
}
