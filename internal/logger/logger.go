package logger

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

// Constants defining log levels
const (
	DebugLevel LogLevel = iota // 0
	InfoLevel                  // 1
	WarnLevel                  // 2
	ErrorLevel                 // 3
)

// LoggerConfig contains configuration options for the logger
type LoggerConfig struct {
	Level         LogLevel // Minimum level to log
	OutputType    string   // "console" or "file"
	LogDir        string   // Directory for log files (required for file output)
	FilePrefix    string   // Prefix for log file names (required for file output)
	RetentionDays int      // Number of days to keep log files (default: 7)
}

// Logger represents a logging instance
type Logger struct {
	mu            sync.Mutex
	level         LogLevel
	outputType    string
	logDir        string
	filePrefix    string
	retentionDays int
	logger        *log.Logger // Single logger instance
	currentFile   *os.File
	ctx           context.Context    // Context for managing goroutine lifecycle
	cancel        context.CancelFunc // Cancel function to stop goroutines
}

var (
	globalLogger *Logger
	globalMu     sync.Mutex
)

// InitGlobalLogger initializes the global logger with the provided configuration
func InitGlobalLogger(config LoggerConfig) (*Logger, error) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalLogger != nil {
		return globalLogger, nil
	}

	// Set default output type if not specified
	if config.OutputType == "" {
		config.OutputType = "console"
	}

	// Validate output type
	if config.OutputType != "console" && config.OutputType != "file" {
		return nil, fmt.Errorf("invalid output type: %s. Must be 'console' or 'file'", config.OutputType)
	}

	// Set default retention days if not specified
	if config.RetentionDays <= 0 {
		config.RetentionDays = 7
	}

	// Validate file configuration if needed
	if config.OutputType == "file" {
		if config.LogDir == "" {
			return nil, errors.New("log directory is required for file output")
		}
		if config.FilePrefix == "" {
			return nil, errors.New("file prefix is required for file output")
		}

		// Create log directory if it doesn't exist
		if err := os.MkdirAll(config.LogDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}
	}

	// Create context for managing goroutine lifecycle
	ctx, cancel := context.WithCancel(context.Background())

	// Create new logger instance
	logger := &Logger{
		level:         config.Level,
		outputType:    config.OutputType,
		logDir:        config.LogDir,
		filePrefix:    config.FilePrefix,
		retentionDays: config.RetentionDays,
		ctx:           ctx,
		cancel:        cancel,
	}

	// Initialize logger based on output type
	if config.OutputType == "console" {
		logger.logger = log.New(os.Stdout, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	} else if config.OutputType == "file" {
		if err := logger.setupFileLogger(); err != nil {
			cancel() // Cleanup if initialization fails
			return nil, err
		}

		// Schedule daily rotation in a new goroutine
		logger.scheduleDailyTasks()

		// Clean up old logs immediately on initialization
		if err := logger.cleanupOldLogs(); err != nil {
			cancel() // Cleanup if cleanup fails
			return nil, fmt.Errorf("failed to clean up old logs: %v", err)
		}
	}

	globalLogger = logger
	return logger, nil
}

// setupFileLogger initializes or rotates the file logger
func (l *Logger) setupFileLogger() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close current file if it exists
	if l.currentFile != nil {
		if err := l.currentFile.Close(); err != nil {
			log.Printf("Warning: failed to close log file: %v", err)
		}
		l.currentFile = nil
	}

	// Create log file name with current date
	dateStr := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("%s_%s.log", l.filePrefix, dateStr)
	filePath := filepath.Join(l.logDir, filename)

	// Open log file in append mode
	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %v", err)
	}

	l.currentFile = file
	l.logger = log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds|log.Lshortfile)
	return nil
}

// scheduleDailyTasks sets up daily log rotation in a separate goroutine
func (l *Logger) scheduleDailyTasks() {
	// Calculate time until next midnight
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	durationUntilMidnight := nextMidnight.Sub(now)

	// Run in a new goroutine to prevent blocking
	go func(ctx context.Context) {
		rotatorTicker := time.NewTicker(durationUntilMidnight)
		defer rotatorTicker.Stop()

		for {
			select {
			case <-rotatorTicker.C:
				l.rotateAndCleanup()
				rotatorTicker.Reset(24 * time.Hour)
			case <-ctx.Done():
				return // Exit when context is cancelled
			}
		}
	}(l.ctx)
}

// rotateAndCleanup handles log rotation and old log cleanup
func (l *Logger) rotateAndCleanup() {
	// Rotate log file
	if err := l.setupFileLogger(); err != nil {
		log.Printf("Error rotating log file: %v", err)
	}

	// Clean up old logs
	if err := l.cleanupOldLogs(); err != nil {
		log.Printf("Error cleaning up old logs: %v", err)
	}
}

// cleanupOldLogs removes log files older than retentionDays
func (l *Logger) cleanupOldLogs() error {
	files, err := os.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %v", err)
	}

	now := time.Now()
	cutoffTime := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -l.retentionDays)
	prefix := l.filePrefix + "_"
	suffix := ".log"

	var oldFiles []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix) {
			fileInfo, err := file.Info()
			if err != nil {
				log.Printf("Warning: failed to get file info for %s: %v", name, err)
				continue
			}

			if fileInfo.ModTime().Before(cutoffTime) {
				oldFiles = append(oldFiles, name)
			}
		}
	}

	// Sort old files by age
	sort.Slice(oldFiles, func(i, j int) bool {
		fileI, _ := os.Stat(filepath.Join(l.logDir, oldFiles[i]))
		fileJ, _ := os.Stat(filepath.Join(l.logDir, oldFiles[j]))
		return fileI.ModTime().Before(fileJ.ModTime())
	})

	// Delete old files
	for _, file := range oldFiles {
		filePath := filepath.Join(l.logDir, file)
		if err := os.Remove(filePath); err != nil {
			log.Printf("Warning: failed to delete old log file %s: %v", filePath, err)
		} else {
			l.log("INFO", "Deleted old log file: %s", filePath)
		}
	}

	return nil
}

// log writes a message with the specified level
func (l *Logger) log(level string, format string, v ...interface{}) {
	if l.logger == nil {
		return
	}
	msg := fmt.Sprintf("[%s] "+format, append([]interface{}{level}, v...)...)
	l.logger.Output(4, msg)
}

// Debug logs a debug level message
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level > DebugLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log("DEBUG", format, v...)
}

// Info logs an info level message
func (l *Logger) Info(format string, v ...interface{}) {
	if l.level > InfoLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log("INFO", format, v...)
}

// Warn logs a warning level message
func (l *Logger) Warn(format string, v ...interface{}) {
	if l.level > WarnLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log("WARN", format, v...)
}

// Error logs an error level message
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level > ErrorLevel {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.log("ERROR", format, v...)
}

// SetLevel changes the log level dynamically
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// Close cleans up resources and stops all goroutines
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.cancel != nil {
		l.cancel()
	}

	if l.currentFile != nil {
		if err := l.currentFile.Close(); err != nil {
			log.Printf("Warning: failed to close log file: %v", err)
		}
		l.currentFile = nil
	}
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	globalMu.Lock()
	defer globalMu.Unlock()
	return globalLogger
}

// ResetGlobalLogger resets the global logger (for testing)
func ResetGlobalLogger() {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.Close()
		globalLogger = nil
	}
}

// Global logger convenience functions
func Debug(format string, v ...interface{}) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.Debug(format, v...)
	}
}

func Info(format string, v ...interface{}) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.Info(format, v...)
	}
}

func Warn(format string, v ...interface{}) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.Warn(format, v...)
	}
}

func Error(format string, v ...interface{}) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.Error(format, v...)
	}
}

// SetLevel sets the global logger level
func SetLevel(level LogLevel) {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalLogger != nil {
		globalLogger.SetLevel(level)
	}
}
