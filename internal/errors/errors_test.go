package errors

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	err := New(ErrorTypeDocker, "test error", nil)
	
	if err.Type != ErrorTypeDocker {
		t.Errorf("expected type %s, got %s", ErrorTypeDocker, err.Type)
	}
	
	if err.Message != "test error" {
		t.Errorf("expected message 'test error', got '%s'", err.Message)
	}
	
	if err.Err != nil {
		t.Errorf("expected nil wrapped error, got %v", err.Err)
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *Error
		expected string
	}{
		{
			name:     "error with wrapped error",
			err:      New(ErrorTypeDocker, "failed to connect", errors.New("connection refused")),
			expected: "[docker] failed to connect: connection refused",
		},
		{
			name:     "error without wrapped error",
			err:      New(ErrorTypeConfig, "invalid config", nil),
			expected: "[config] invalid config",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, got)
			}
		})
	}
}

func TestError_WithContext(t *testing.T) {
	err := New(ErrorTypeContainer, "container error", nil)
	err = err.WithContext("container_id", "abc123")
	
	if len(err.Context) != 1 {
		t.Errorf("expected 1 context entry, got %d", len(err.Context))
	}
	
	if err.Context["container_id"] != "abc123" {
		t.Errorf("expected container_id=abc123, got %v", err.Context["container_id"])
	}
	
	// Chain context
	err = err.WithContext("network_id", "net456")
	
	if len(err.Context) != 2 {
		t.Errorf("expected 2 context entries, got %d", len(err.Context))
	}
}

func TestError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := New(ErrorTypeNginx, "nginx error", innerErr)
	
	unwrapped := err.Unwrap()
	if unwrapped != innerErr {
		t.Errorf("expected unwrapped error to be innerErr, got %v", unwrapped)
	}
}

func TestIsErrorType(t *testing.T) {
	err := New(ErrorTypeSSL, "ssl error", nil)
	
	if !IsErrorType(err, ErrorTypeSSL) {
		t.Error("expected IsErrorType to return true for ErrorTypeSSL")
	}
	
	if IsErrorType(err, ErrorTypeDocker) {
		t.Error("expected IsErrorType to return false for ErrorTypeDocker")
	}
	
	// Test with non-Error type
	otherErr := errors.New("other error")
	if IsErrorType(otherErr, ErrorTypeSSL) {
		t.Error("expected IsErrorType to return false for non-Error type")
	}
}

func TestGetErrorContext(t *testing.T) {
	err := New(ErrorTypeNetwork, "network error", nil).
		WithContext("network_id", "net123").
		WithContext("container_id", "cont456")
	
	ctx := GetErrorContext(err)
	
	if len(ctx) != 2 {
		t.Errorf("expected 2 context entries, got %d", len(ctx))
	}
	
	if ctx["network_id"] != "net123" {
		t.Errorf("expected network_id=net123, got %v", ctx["network_id"])
	}
	
	// Test with non-Error type
	otherErr := errors.New("other error")
	ctx = GetErrorContext(otherErr)
	if ctx != nil {
		t.Errorf("expected nil context for non-Error type, got %v", ctx)
	}
}

func TestWrap(t *testing.T) {
	innerErr := errors.New("inner error")
	wrapped := Wrap(innerErr, ErrorTypeSystem, "system error")
	
	customErr, ok := wrapped.(*Error)
	if !ok {
		t.Fatal("expected wrapped error to be *Error type")
	}
	
	if customErr.Type != ErrorTypeSystem {
		t.Errorf("expected type %s, got %s", ErrorTypeSystem, customErr.Type)
	}
	
	if customErr.Message != "system error" {
		t.Errorf("expected message 'system error', got '%s'", customErr.Message)
	}
	
	if customErr.Err != innerErr {
		t.Errorf("expected wrapped error to be innerErr, got %v", customErr.Err)
	}
	
	// Test wrapping nil
	wrapped = Wrap(nil, ErrorTypeSystem, "should be nil")
	if wrapped != nil {
		t.Errorf("expected nil when wrapping nil error, got %v", wrapped)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{
			name:      "docker error is retryable",
			err:       New(ErrorTypeDocker, "docker error", nil),
			retryable: true,
		},
		{
			name:      "network error is retryable",
			err:       New(ErrorTypeNetwork, "network error", nil),
			retryable: true,
		},
		{
			name:      "config error is not retryable",
			err:       New(ErrorTypeConfig, "config error", nil),
			retryable: false,
		},
		{
			name:      "nginx error is not retryable",
			err:       New(ErrorTypeNginx, "nginx error", nil),
			retryable: false,
		},
		{
			name:      "nil error is not retryable",
			err:       nil,
			retryable: false,
		},
		{
			name:      "standard error is not retryable",
			err:       errors.New("standard error"),
			retryable: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryableError(tt.err)
			if got != tt.retryable {
				t.Errorf("expected retryable=%v, got %v", tt.retryable, got)
			}
		})
	}
}

func TestRetry(t *testing.T) {
	t.Run("successful on first attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return nil
		}
		
		config := &RetryConfig{
			MaxAttempts: 3,
			Delay:       10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
			Backoff:     2.0,
		}
		
		err := Retry(context.Background(), config, operation)
		
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		
		if attempts != 1 {
			t.Errorf("expected 1 attempt, got %d", attempts)
		}
	})
	
	t.Run("successful on third attempt", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			if attempts < 3 {
				return errors.New("temporary error")
			}
			return nil
		}
		
		config := &RetryConfig{
			MaxAttempts: 3,
			Delay:       10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
			Backoff:     2.0,
		}
		
		err := Retry(context.Background(), config, operation)
		
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})
	
	t.Run("fails after max attempts", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			return errors.New("persistent error")
		}
		
		config := &RetryConfig{
			MaxAttempts: 3,
			Delay:       10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
			Backoff:     2.0,
		}
		
		err := Retry(context.Background(), config, operation)
		
		if err == nil {
			t.Error("expected error after max attempts")
		}
		
		if attempts != 3 {
			t.Errorf("expected 3 attempts, got %d", attempts)
		}
	})
	
	t.Run("context cancellation", func(t *testing.T) {
		attempts := 0
		operation := func() error {
			attempts++
			time.Sleep(50 * time.Millisecond)
			return errors.New("error")
		}
		
		config := &RetryConfig{
			MaxAttempts: 5,
			Delay:       10 * time.Millisecond,
			MaxDelay:    100 * time.Millisecond,
			Backoff:     2.0,
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		
		err := Retry(ctx, config, operation)
		
		if err == nil {
			t.Error("expected error due to context cancellation")
		}
		
		if !IsErrorType(err, ErrorTypeSystem) {
			t.Errorf("expected system error, got %T", err)
		}
	})
}

func TestDefaultRetryConfig(t *testing.T) {
	config := DefaultRetryConfig()
	
	if config.MaxAttempts != 3 {
		t.Errorf("expected MaxAttempts=3, got %d", config.MaxAttempts)
	}
	
	if config.Delay != time.Second {
		t.Errorf("expected Delay=1s, got %v", config.Delay)
	}
	
	if config.MaxDelay != 30*time.Second {
		t.Errorf("expected MaxDelay=30s, got %v", config.MaxDelay)
	}
	
	if config.Backoff != 2.0 {
		t.Errorf("expected Backoff=2.0, got %f", config.Backoff)
	}
}
