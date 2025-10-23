package nginx

// Controller defines the behaviour required by consumers to manage nginx config.
type Controller interface {
	UpdateConfig(config string) error
}
