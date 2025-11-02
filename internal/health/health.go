package health

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the overall health status
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

// Check represents a single health check
type Check struct {
	Name    string        `json:"name"`
	Status  Status        `json:"status"`
	Message string        `json:"message,omitempty"`
	Latency time.Duration `json:"latency_ms"`
}

// Response represents the health check response
type Response struct {
	Status    Status            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Uptime    time.Duration     `json:"uptime_seconds"`
	Checks    []Check           `json:"checks"`
	Metrics   map[string]interface{} `json:"metrics,omitempty"`
}

// Checker defines the interface for health checks
type Checker interface {
	Check() Check
}

// Manager manages health checks and provides HTTP endpoint
type Manager struct {
	startTime time.Time
	checkers  []Checker
	metrics   map[string]interface{}
	mu        sync.RWMutex
}

// NewManager creates a new health check manager
func NewManager() *Manager {
	return &Manager{
		startTime: time.Now(),
		checkers:  make([]Checker, 0),
		metrics:   make(map[string]interface{}),
	}
}

// RegisterChecker registers a new health checker
func (m *Manager) RegisterChecker(checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checkers = append(m.checkers, checker)
}

// UpdateMetric updates a metric value
func (m *Manager) UpdateMetric(name string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.metrics[name] = value
}

// GetStatus performs all health checks and returns the status
func (m *Manager) GetStatus() Response {
	m.mu.RLock()
	defer m.mu.RUnlock()

	checks := make([]Check, 0, len(m.checkers))
	overallStatus := StatusHealthy

	for _, checker := range m.checkers {
		check := checker.Check()
		checks = append(checks, check)

		// Determine overall status (worst wins)
		if check.Status == StatusUnhealthy {
			overallStatus = StatusUnhealthy
		} else if check.Status == StatusDegraded && overallStatus != StatusUnhealthy {
			overallStatus = StatusDegraded
		}
	}

	// Copy metrics
	metricsCopy := make(map[string]interface{}, len(m.metrics))
	for k, v := range m.metrics {
		metricsCopy[k] = v
	}

	return Response{
		Status:    overallStatus,
		Timestamp: time.Now(),
		Uptime:    time.Since(m.startTime),
		Checks:    checks,
		Metrics:   metricsCopy,
	}
}

// Handler returns an HTTP handler for health checks
func (m *Manager) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := m.GetStatus()

		w.Header().Set("Content-Type", "application/json")
		
		// Set HTTP status code based on health status
		switch status.Status {
		case StatusHealthy:
			w.WriteHeader(http.StatusOK)
		case StatusDegraded:
			w.WriteHeader(http.StatusOK) // 200 but with degraded status
		case StatusUnhealthy:
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		if err := json.NewEncoder(w).Encode(status); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}
}

// ReadinessHandler returns a simple readiness check (no details)
func (m *Manager) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := m.GetStatus()
		
		if status.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
			return
		}
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ready"))
	}
}

// LivenessHandler returns a simple liveness check
func (m *Manager) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Simple liveness - just respond if the service is running
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("alive"))
	}
}
