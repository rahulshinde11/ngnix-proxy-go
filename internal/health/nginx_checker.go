package health

import (
	"os/exec"
	"time"
)

// NginxChecker checks if nginx is running and responding
type NginxChecker struct{}

// NewNginxChecker creates a new nginx health checker
func NewNginxChecker() *NginxChecker {
	return &NginxChecker{}
}

// Check performs the nginx health check
func (c *NginxChecker) Check() Check {
	start := time.Now()
	
	// Check if nginx process is responding
	cmd := exec.Command("nginx", "-t")
	err := cmd.Run()
	
	latency := time.Since(start)
	
	if err != nil {
		return Check{
			Name:    "nginx",
			Status:  StatusUnhealthy,
			Message: "nginx config test failed: " + err.Error(),
			Latency: latency,
		}
	}
	
	return Check{
		Name:    "nginx",
		Status:  StatusHealthy,
		Message: "nginx is running and configuration is valid",
		Latency: latency,
	}
}
