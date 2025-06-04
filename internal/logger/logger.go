package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/rahulshinde/nginx-proxy-go/internal/errors"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	// DebugLevel represents debug messages
	DebugLevel LogLevel = iota
	// InfoLevel represents informational messages
	InfoLevel
	// WarnLevel represents warning messages
	WarnLevel
	// ErrorLevel represents error messages
	ErrorLevel
	// FatalLevel represents fatal messages
	FatalLevel
)

var (
	levelNames = map[LogLevel]string{
		DebugLevel: "DEBUG",
		InfoLevel:  "INFO",
		WarnLevel:  "WARN",
		ErrorLevel: "ERROR",
		FatalLevel: "FATAL",
	}
)

// Config represents the logger configuration
type Config struct {
	Level      LogLevel
	OutputPath string
	MaxSize    int    // Maximum size in megabytes
	MaxBackups int    // Maximum number of old log files to retain
	MaxAge     int    // Maximum number of days to retain old log files
	Compress   bool   // Whether to compress rotated log files
	Format     string // Log format (json or text)
}

// DefaultConfig returns the default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:      InfoLevel,
		OutputPath: "/var/log/nginx-proxy/nginx-proxy.log",
		MaxSize:    100,    // 100MB
		MaxBackups: 3,      // Keep 3 backup files
		MaxAge:     7,      // Keep logs for 7 days
		Compress:   true,   // Compress rotated logs
		Format:     "text", // Default to text format
	}
}

// Logger represents a logger instance
type Logger struct {
	config *Config
	mu     sync.Mutex
	file   *os.File
	level  LogLevel
	format string
}

// New creates a new logger instance
func New(config *Config) (*Logger, error) {
	// Create log directory if it doesn't exist
	dir := filepath.Dir(config.OutputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, errors.New(errors.ErrorTypeSystem, "failed to create log directory", err)
	}

	// Open log file
	file, err := os.OpenFile(config.OutputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.New(errors.ErrorTypeSystem, "failed to open log file", err)
	}

	return &Logger{
		config: config,
		file:   file,
		level:  config.Level,
		format: config.Format,
	}, nil
}

// Close closes the logger
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// rotate rotates the log file if necessary
func (l *Logger) rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Check if rotation is needed
	info, err := l.file.Stat()
	if err != nil {
		return errors.New(errors.ErrorTypeSystem, "failed to get file info", err)
	}

	if info.Size() < int64(l.config.MaxSize*1024*1024) {
		return nil
	}

	// Close current file
	if err := l.file.Close(); err != nil {
		return errors.New(errors.ErrorTypeSystem, "failed to close log file", err)
	}

	// Rotate old log files
	for i := l.config.MaxBackups - 1; i > 0; i-- {
		oldPath := fmt.Sprintf("%s.%d", l.config.OutputPath, i)
		newPath := fmt.Sprintf("%s.%d", l.config.OutputPath, i+1)
		if err := os.Rename(oldPath, newPath); err != nil && !os.IsNotExist(err) {
			return errors.New(errors.ErrorTypeSystem, "failed to rotate log file", err)
		}
	}

	// Rename current log file
	if err := os.Rename(l.config.OutputPath, l.config.OutputPath+".1"); err != nil {
		return errors.New(errors.ErrorTypeSystem, "failed to rename log file", err)
	}

	// Open new log file
	file, err := os.OpenFile(l.config.OutputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return errors.New(errors.ErrorTypeSystem, "failed to open new log file", err)
	}

	l.file = file
	return nil
}

// log writes a log message
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	// Rotate log file if necessary
	if err := l.rotate(); err != nil {
		log.Printf("Failed to rotate log file: %v", err)
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "unknown"
		line = 0
	}

	// Format message
	message := fmt.Sprintf(format, args...)
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	levelName := levelNames[level]

	// Format log entry
	var entry string
	if l.format == "json" {
		entry = fmt.Sprintf(`{"timestamp":"%s","level":"%s","file":"%s","line":%d,"message":"%s"}`,
			timestamp, levelName, filepath.Base(file), line, message)
	} else {
		entry = fmt.Sprintf("[%s] %s [%s:%d] %s",
			timestamp, levelName, filepath.Base(file), line, message)
	}

	// Write to file
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := fmt.Fprintln(l.file, entry); err != nil {
		log.Printf("Failed to write to log file: %v", err)
	}

	// Also write to stdout for development
	if level >= ErrorLevel {
		fmt.Fprintln(os.Stderr, entry)
	} else {
		fmt.Fprintln(os.Stdout, entry)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DebugLevel, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(InfoLevel, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WarnLevel, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ErrorLevel, format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FatalLevel, format, args...)
	os.Exit(1)
}

// WithError logs an error with context
func (l *Logger) WithError(err error) *Logger {
	if e, ok := err.(*errors.Error); ok {
		context := errors.GetErrorContext(err)
		if len(context) > 0 {
			l.Error("[%s] %s: %v (context: %v)", e.Type, e.Message, e.Err, context)
		} else {
			l.Error("[%s] %s: %v", e.Type, e.Message, e.Err)
		}
	} else {
		l.Error("%v", err)
	}
	return l
}

// SetLevel sets the log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetOutput sets the log output
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.file = w.(*os.File)
}

// SetFormat sets the log format
func (l *Logger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if format == "json" || format == "text" {
		l.format = format
	}
}
