package logger

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestInitGlobalLogger tests the logger initialization functionality
func TestInitGlobalLogger(t *testing.T) {
	// Reset global logger before test
	ResetGlobalLogger()

	// Test default console output
	_, err := InitGlobalLogger(LoggerConfig{})
	if err != nil {
		t.Fatalf("Failed to initialize default logger: %v", err)
	}

	// Test repeated initialization
	logger1, err := InitGlobalLogger(LoggerConfig{})
	if err != nil {
		t.Fatalf("Failed to initialize logger repeatedly: %v", err)
	}
	logger2 := GetLogger()
	if logger1 != logger2 {
		t.Error("Repeated initialization should return the same instance")
	}

	// Test invalid output type
	ResetGlobalLogger()
	_, err = InitGlobalLogger(LoggerConfig{OutputType: "invalid"})
	if err == nil {
		t.Error("Invalid output type should not return nil")
	}

	// Test file output with missing required configuration
	_, err = InitGlobalLogger(LoggerConfig{OutputType: "file"})
	if err == nil {
		t.Error("File output with missing directory should return error")
	}

	// Test valid file output configuration
	tempDir := t.TempDir()
	_, err = InitGlobalLogger(LoggerConfig{
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "test",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize file logger: %v", err)
	}
}

// TestLogLevels tests log level filtering functionality
func TestLogLevels(t *testing.T) {
	ResetGlobalLogger()
	tempDir := t.TempDir()

	// Initialize logger with Info level
	logger, err := InitGlobalLogger(LoggerConfig{
		Level:         InfoLevel,
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "leveltest",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Test if different log levels behave as expected
	logger.Debug("This debug message should not be output")
	logger.Info("This info message should be output")
	logger.Warn("This warning message should be output")
	logger.Error("This error message should be output")

	// Switch to Error level
	logger.SetLevel(ErrorLevel)
	logger.Info("This info message should not be output")
	logger.Warn("This warning message should not be output")
	logger.Error("This error message should be output")

	// Check log file content
	dateStr := time.Now().Format("2006-01-02")
	logFile := filepath.Join(tempDir, "leveltest_"+dateStr+".log")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	contentStr := string(content)

	if strings.Contains(contentStr, "This debug message should not be output") {
		t.Error("Debug message should not be output at Info level")
	}
	if !strings.Contains(contentStr, "This info message should be output") {
		t.Error("Info message should be output at Info level")
	}
	if !strings.Contains(contentStr, "This error message should be output") {
		t.Error("Error message should be output")
	}
}

// TestLogRotation tests log rotation functionality (simulated)
func TestLogRotation(t *testing.T) {
	ResetGlobalLogger()
	tempDir := t.TempDir()

	logger, err := InitGlobalLogger(LoggerConfig{
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "rotationtest",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Log a message
	logger.Info("Log before rotation test")

	// Manually trigger log rotation
	logger.rotateAndCleanup()

	// Log after rotation
	logger.Info("Log after rotation test")

	// Check if new log file was created (should be same file on same day)
	dateStr := time.Now().Format("2006-01-02")
	logFile := filepath.Join(tempDir, "rotationtest_"+dateStr+".log")

	if _, err := os.Stat(logFile); os.IsNotExist(err) {
		t.Fatalf("Log file does not exist: %v", err)
	}
}

// TestLogCleanup tests old log cleanup functionality with modified file times
func TestLogCleanup(t *testing.T) {
	ResetGlobalLogger()
	tempDir := t.TempDir()

	// Create test log files with specific timestamps
	// We'll create files with timestamps relative to current time
	testFiles := []struct {
		filename     string
		ageInDays    int
		shouldBeKept bool
	}{
		{"cleantest_2025-07-01.log", 3, false}, // 3 days old - should be deleted
		{"cleantest_2023-07-02.log", 2, false}, // 2 days old - should be deleted
		{"cleantest_2023-07-03.log", 1, true},  // 1 day old - should be kept
		{"cleantest_2023-07-04.log", 0, true},  // Today - should be kept
		{"otherfile.log", 3, true},             // Different pattern - should be kept
	}

	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.filename)
		f, err := os.Create(filePath)
		if err != nil {
			t.Fatalf("Failed to create test log file %s: %v", tf.filename, err)
		}
		f.WriteString("Test log content")
		f.Close()

		// Set custom modification time based on age
		modTime := time.Now().AddDate(0, 0, -tf.ageInDays)
		if err := os.Chtimes(filePath, modTime, modTime); err != nil {
			t.Fatalf("Failed to set modification time for %s: %v", tf.filename, err)
		}
	}

	// Initialize logger with 1 day retention
	logger, err := InitGlobalLogger(LoggerConfig{
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "cleantest",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Manually trigger cleanup
	logger.cleanupOldLogs()

	// Verify cleanup results
	for _, tf := range testFiles {
		filePath := filepath.Join(tempDir, tf.filename)
		_, err := os.Stat(filePath)

		if tf.shouldBeKept {
			if os.IsNotExist(err) {
				t.Errorf("Log file %s should be kept but was deleted", tf.filename)
			}
		} else {
			if !os.IsNotExist(err) {
				t.Errorf("Log file %s should be deleted but still exists", tf.filename)
			}
		}
	}
}

// TestConcurrentLogging tests concurrent log writing
func TestConcurrentLogging(t *testing.T) {
	ResetGlobalLogger()
	tempDir := t.TempDir()

	logger, err := InitGlobalLogger(LoggerConfig{
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "concurrenttest",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Concurrent log writing
	const goroutines = 10
	const logsPerGoroutine = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < logsPerGoroutine; j++ {
				logger.Info("Concurrent log test - Goroutine %d, Log %d", id, j)
			}
		}(i)
	}

	wg.Wait()

	// Check if log file exists and has content
	dateStr := time.Now().Format("2006-01-02")
	logFile := filepath.Join(tempDir, "concurrenttest_"+dateStr+".log")

	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Simple verification of log count (not exact but checks general correctness)
	contentStr := string(content)
	if count := strings.Count(contentStr, "Concurrent log test"); count < goroutines*logsPerGoroutine*9/10 {
		t.Errorf("Insufficient log entries, expected at least %d, got %d", goroutines*logsPerGoroutine, count)
	}
}

// TestClose tests logger closing functionality
func TestClose(t *testing.T) {
	ResetGlobalLogger()
	tempDir := t.TempDir()

	logger, err := InitGlobalLogger(LoggerConfig{
		OutputType:    "file",
		LogDir:        tempDir,
		FilePrefix:    "closetest",
		RetentionDays: 1,
	})
	if err != nil {
		t.Fatalf("Failed to initialize logger: %v", err)
	}

	// Close the logger
	logger.Close()

	// Attempt to write log after closing (should not panic)
	logger.Info("Log written after closing")

	// Check if global logger is still accessible
	globalLogger := GetLogger()
	if globalLogger == nil {
		t.Error("Global logger should not be nil after closing")
	}
}
