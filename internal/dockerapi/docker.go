package dockerapi

import (
	"context"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
)

// Client exposes the Docker operations required by the application.
type Client interface {
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	Events(ctx context.Context, options types.EventsOptions) (<-chan events.Message, <-chan error)
	NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error)
}

type clientAdapter struct {
	inner *client.Client
}

// New wraps a Docker client to satisfy the Client interface.
func New(inner *client.Client) Client {
	return &clientAdapter{inner: inner}
}

func (c *clientAdapter) ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error) {
	return c.inner.ContainerInspect(ctx, containerID)
}

func (c *clientAdapter) ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error) {
	return c.inner.ContainerList(ctx, options)
}

func (c *clientAdapter) Events(ctx context.Context, options types.EventsOptions) (<-chan events.Message, <-chan error) {
	return c.inner.Events(ctx, options)
}

func (c *clientAdapter) NetworkInspect(ctx context.Context, networkID string, options types.NetworkInspectOptions) (types.NetworkResource, error) {
	return c.inner.NetworkInspect(ctx, networkID, options)
}
