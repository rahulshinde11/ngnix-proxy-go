package event

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/rahulshinde/nginx-proxy-go/internal/dockerapi"
)

// EventType represents the type of Docker event
type EventType string

const (
	EventTypeContainer EventType = "container"
	EventTypeNetwork   EventType = "network"
	EventTypeService   EventType = "service"
)

// EventHandler defines the interface for handling Docker events
type EventHandler interface {
	HandleContainerEvent(ctx context.Context, event events.Message) error
	HandleNetworkEvent(ctx context.Context, event events.Message) error
	HandleServiceEvent(ctx context.Context, event events.Message) error
}

// Processor handles Docker events with enhanced functionality
type Processor struct {
	client    dockerapi.Client
	handler   EventHandler
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	eventChan chan events.Message
	errChan   chan error
}

// NewProcessor creates a new event processor
func NewProcessor(client dockerapi.Client, handler EventHandler) *Processor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Processor{
		client:    client,
		handler:   handler,
		ctx:       ctx,
		cancel:    cancel,
		eventChan: make(chan events.Message, 100),
		errChan:   make(chan error, 1),
	}
}

// Start begins processing Docker events
func (p *Processor) Start() error {
	return p.StartSince("")
}

// StartSince begins processing Docker events since a specific time
func (p *Processor) StartSince(since string) error {
	options := types.EventsOptions{}
	if since != "" {
		options.Since = since
	}

	// Start event stream
	events, errs := p.client.Events(p.ctx, options)

	// Start event processing goroutine
	go p.processEvents(events, errs)

	return nil
}

// Stop stops the event processor
func (p *Processor) Stop() {
	p.cancel()
}

// processEvents processes incoming Docker events
func (p *Processor) processEvents(eventChan <-chan events.Message, errs <-chan error) {
	for {
		select {
		case err := <-errs:
			if err != nil {
				log.Printf("Error receiving Docker events: %v", err)
				p.errChan <- err
				return
			}
		case event := <-eventChan:
			// Handle events asynchronously to prevent blocking the event stream
			eventCopy := event
			go func() {
				if err := p.handleEvent(eventCopy); err != nil {
					log.Printf("Error handling event: %v", err)
				}
			}()
		case <-p.ctx.Done():
			return
		}
	}
}

// handleEvent routes events to appropriate handlers
func (p *Processor) handleEvent(event events.Message) error {
	// No mutex lock here - let individual handlers manage their own concurrency
	switch EventType(event.Type) {
	case EventTypeContainer:
		return p.handler.HandleContainerEvent(p.ctx, event)
	case EventTypeNetwork:
		return p.handler.HandleNetworkEvent(p.ctx, event)
	case EventTypeService:
		return p.handler.HandleServiceEvent(p.ctx, event)
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}

// GetEventChannel returns the event channel
func (p *Processor) GetEventChannel() <-chan events.Message {
	return p.eventChan
}

// GetErrorChannel returns the error channel
func (p *Processor) GetErrorChannel() <-chan error {
	return p.errChan
}

// IsContainerEvent checks if an event is a container event
func IsContainerEvent(event events.Message) bool {
	return EventType(event.Type) == EventTypeContainer
}

// IsNetworkEvent checks if an event is a network event
func IsNetworkEvent(event events.Message) bool {
	return EventType(event.Type) == EventTypeNetwork
}

// IsServiceEvent checks if an event is a service event
func IsServiceEvent(event events.Message) bool {
	return EventType(event.Type) == EventTypeService
}

// GetContainerID returns the container ID from an event
func GetContainerID(event events.Message) string {
	if IsContainerEvent(event) {
		return event.ID
	}
	if IsNetworkEvent(event) {
		if container, ok := event.Actor.Attributes["container"]; ok {
			return container
		}
	}
	return ""
}

// GetNetworkID returns the network ID from an event
func GetNetworkID(event events.Message) string {
	if IsNetworkEvent(event) {
		return event.Actor.ID
	}
	return ""
}

// GetEventScope returns the event scope
func GetEventScope(event events.Message) string {
	return event.Scope
}

// GetEventAction returns the event action
func GetEventAction(event events.Message) string {
	return string(event.Action)
}
