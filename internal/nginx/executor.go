package nginx

import "os/exec"

// Commander abstracts command execution for nginx operations.
type Commander interface {
	Command(name string, arg ...string) Cmd
}

// Cmd represents an executable command.
type Cmd interface {
	CombinedOutput() ([]byte, error)
	Start() error
}

type defaultCommander struct{}

// DefaultCommander uses os/exec to run commands.
var DefaultCommander Commander = &defaultCommander{}

func (*defaultCommander) Command(name string, arg ...string) Cmd {
	return &execCmdWrapper{Cmd: exec.Command(name, arg...)}
}

type execCmdWrapper struct {
	*exec.Cmd
}

func (c *execCmdWrapper) CombinedOutput() ([]byte, error) {
	return c.Cmd.CombinedOutput()
}

func (c *execCmdWrapper) Start() error {
	return c.Cmd.Start()
}
