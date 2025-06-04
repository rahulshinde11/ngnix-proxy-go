package acme

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// DNSProvider represents a DNS provider for ACME DNS challenges
type DNSProvider interface {
	CreateRecord(domain, name, value string) (string, error)
	DeleteRecord(domain, recordID string) error
}

// DigitalOcean represents a DigitalOcean DNS provider
type DigitalOcean struct {
	apiToken string
	client   *http.Client
}

// NewDigitalOcean creates a new DigitalOcean DNS provider
func NewDigitalOcean() *DigitalOcean {
	return &DigitalOcean{
		apiToken: os.Getenv("DO_API_TOKEN"),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CreateRecord creates a DNS record
func (d *DigitalOcean) CreateRecord(domain, name, value string) (string, error) {
	payload := map[string]interface{}{
		"type":     "TXT",
		"name":     name,
		"data":     value,
		"ttl":      60,
		"flags":    nil,
		"tag":      nil,
		"port":     nil,
		"weight":   nil,
		"priority": nil,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %v", err)
	}

	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records", domain)
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiToken)

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("failed to create record: %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %v", err)
	}

	record := result["domain_record"].(map[string]interface{})
	return fmt.Sprintf("%v", record["id"]), nil
}

// DeleteRecord deletes a DNS record
func (d *DigitalOcean) DeleteRecord(domain, recordID string) error {
	url := fmt.Sprintf("https://api.digitalocean.com/v2/domains/%s/records/%s", domain, recordID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Authorization", "Bearer "+d.apiToken)

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete record: %d", resp.StatusCode)
	}

	return nil
}
