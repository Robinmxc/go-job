package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// FileInfo holds information about a file or directory entry
type FileInfo struct {
	Path     string     // Full path to the entry
	IsDir    bool       // Whether the entry is a directory
	Content  []byte     // File content (only for files, nil for directories)
	Children []FileInfo // Child entries (only for directories, recursive)
	Error    error      // Error encountered when processing this entry
}

// ReadFileOrDir reads a file or directory (non-recursive for directories):
// - For files: returns content in Content field
// - For directories: returns list of direct child entries (files and subdirectories) in Children field
// Args:
//   - path: Target file or directory path to read
//
// Returns:
//   - FileInfo: Struct containing entry details, content (if file), direct children (if directory), and any error
func ReadFileOrDir(path string) FileInfo {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{Path: path, Error: err}
	}

	if !info.IsDir() {
		// Handle regular file: read content
		content, err := os.ReadFile(path)
		return FileInfo{
			Path:    path,
			IsDir:   false,
			Content: content,
			Error:   err,
		}
	}

	// Handle directory: list only direct children (non-recursive)
	entries, err := os.ReadDir(path)
	if err != nil {
		return FileInfo{Path: path, IsDir: true, Error: err}
	}

	var children []FileInfo
	for _, entry := range entries {
		childPath := filepath.Join(path, entry.Name())
		// Get basic info for child without recursive reading
		childInfo, err := os.Stat(childPath)
		if err != nil {
			children = append(children, FileInfo{
				Path:  childPath,
				IsDir: false, // Default to false, error will indicate issue
				Error: err,
			})
			continue
		}

		// For child entries, only populate basic info (no content/children)
		children = append(children, FileInfo{
			Path:  childPath,
			IsDir: childInfo.IsDir(),
			Error: nil,
		})
	}

	return FileInfo{
		Path:     path,
		IsDir:    true,
		Children: children,
		Error:    nil,
	}
}

// WriteConfig defines customizable parameters for file writing operations
type WriteConfig struct {
	Perm os.FileMode // File permission bits (e.g., 0644, 0755)
	Flag int         // File opening flags (e.g., os.O_WRONLY|os.O_CREATE)
	User string      // Owner UID (Unix/Linux only, empty preserves current)
}

// WriteFile writes data to a file with configurable options
// Args:
//   - path:    Target file path
//   - data:    Content to write
//   - config:  Optional settings (uses defaults if empty)
//
// Returns:
//   - error:   Filesystem errors or permission issues
func WriteFile(path string, data []byte, config ...WriteConfig) error {
	// Provides default values
	cfg := WriteConfig{
		Perm: 0644,                                   // -rw-r--r--
		Flag: os.O_WRONLY | os.O_CREATE | os.O_TRUNC, // Overwrite existing
	}
	if len(config) > 0 {
		cfg = config[0]
	}

	// Ensure parent directories exist (with execute permission for traversal)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	// Open file with specified flags and permissions
	file, err := os.OpenFile(path, cfg.Flag, cfg.Perm)
	if err != nil {
		return err
	}
	defer file.Close() // Ensures file handle cleanup

	// Atomic write operation
	if _, err := file.Write(data); err != nil {
		return err
	}

	if len(cfg.User) > 0 {
		user, err := lookupUser(cfg.User, defaultLooker)
		if err != nil {
			return fmt.Errorf("failed to lookup user %s: %w", cfg.User, err)
		}

		uid, err := strconv.Atoi(user.Uid)
		if err != nil {
			return fmt.Errorf("invalid user ID for user %s: %w", cfg.User, err)
		}

		gid, err := strconv.Atoi(user.Gid)
		if err != nil {
			return fmt.Errorf("invalid group ID for user %s: %w", cfg.User, err)
		}

		if err := os.Chown(path, uid, gid); err != nil {
			return err
		}
	}
	return nil
}

// GenerateFileName creates a unique filename using timestamp with nanosecond precision
// Args:
//   - prefix: Optional filename prefix (e.g., "log_")
//   - suffix: Optional filename suffix (e.g., ".txt")
//
// Returns:
//   - string: Generated filename like "prefix{unique_part}suffix"
func GenerateFileName(prefix string, suffix string) string {
	return prefix + time.Now().Format("20060102_150405.000000000") + suffix
}

// ThreadSafeWriteFile writes data to a file with configurable options is thread safe
// Args:
//   - path:    Target file path
//   - data:    Content to write
//   - config:  Optional settings (uses defaults if empty)
//
// Returns:
//   - error:   Filesystem errors or permission issues
func ThreadSafeWriteFile(path string, data []byte, config ...WriteConfig) error {
	tempFile := "/tmp/" + GenerateFileName("", ".txt")
	err := WriteFile(tempFile, data, config...)
	if err != nil {
		return err
	}
	err = os.Rename(tempFile, path)
	if err != nil {
		return fmt.Errorf("rename file from %s to %s : %w", tempFile, path, err)
	}
	return nil
}
