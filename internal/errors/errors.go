package errors

import (
	"context"
	"fmt"
	"log"
	"time"
)

// ErrorType represents the category of an error
type ErrorType string

const (
	// ErrorTypeDocker represents errors from Docker operations
	ErrorTypeDocker ErrorType = "docker"
	// ErrorTypeNginx represents errors from Nginx operations
	ErrorTypeNginx ErrorType = "nginx"
	// ErrorTypeConfig represents errors from configuration operations
	ErrorTypeConfig ErrorType = "config"
	// ErrorTypeNetwork represents errors from network operations
	ErrorTypeNetwork ErrorType = "network"
	// ErrorTypeContainer represents errors from container operations
	ErrorTypeContainer ErrorType = "container"
	// ErrorTypeSSL represents errors from SSL operations
	ErrorTypeSSL ErrorType = "ssl"
	// ErrorTypeSystem represents system-level errors
	ErrorTypeSystem ErrorType = "system"
)

// Error represents a custom error with additional context
type Error struct {
	Type    ErrorType
	Message string
	Err     error
	Context map[string]interface{}
}

// Error implements the error interface
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// New creates a new Error
func New(errType ErrorType, message string, err error) *Error {
	return &Error{
		Type:    errType,
		Message: message,
		Err:     err,
		Context: make(map[string]interface{}),
	}
}

// WithContext adds context to the error
func (e *Error) WithContext(key string, value interface{}) *Error {
	e.Context[key] = value
	return e
}

// RetryConfig represents the configuration for retrying operations
type RetryConfig struct {
	MaxAttempts int
	Delay       time.Duration
	MaxDelay    time.Duration
	Backoff     float64
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts: 3,
		Delay:       time.Second,
		MaxDelay:    time.Second * 30,
		Backoff:     2.0,
	}
}

// Retry executes the given operation with retry logic
func Retry(ctx context.Context, config *RetryConfig, operation func() error) error {
	var err error
	delay := config.Delay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return New(ErrorTypeSystem, "operation cancelled", ctx.Err())
		default:
			err = operation()
			if err == nil {
				return nil
			}

			if attempt == config.MaxAttempts {
				return New(ErrorTypeSystem, fmt.Sprintf("operation failed after %d attempts", attempt), err)
			}

			log.Printf("Attempt %d failed: %v. Retrying in %v...", attempt, err, delay)
			time.Sleep(delay)

			// Calculate next delay with exponential backoff
			delay = time.Duration(float64(delay) * config.Backoff)
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}
		}
	}

	return err
}

// IsRetryableError checks if an error should be retried
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's our custom error type
	if e, ok := err.(*Error); ok {
		switch e.Type {
		case ErrorTypeDocker, ErrorTypeNetwork:
			return true
		case ErrorTypeNginx, ErrorTypeConfig, ErrorTypeContainer, ErrorTypeSSL, ErrorTypeSystem:
			return false
		}
	}

	// Add more retryable error checks here
	return false
}

// Wrap wraps an error with additional context
func Wrap(err error, errType ErrorType, message string) error {
	if err == nil {
		return nil
	}
	return New(errType, message, err)
}

// IsErrorType checks if an error is of a specific type
func IsErrorType(err error, errType ErrorType) bool {
	if e, ok := err.(*Error); ok {
		return e.Type == errType
	}
	return false
}

// GetErrorContext returns the context of an error
func GetErrorContext(err error) map[string]interface{} {
	if e, ok := err.(*Error); ok {
		return e.Context
	}
	return nil
}
