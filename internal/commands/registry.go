package commands

import (
	"context"
	"fmt"
	"strings"
)

type Command struct {
	Name        string
	Description string
	Aliases     []string
	Handler     func(ctx context.Context, args string) (string, error)
}

type Registry struct {
	commands map[string]*Command
}

func NewRegistry() *Registry {
	return &Registry{commands: make(map[string]*Command)}
}

func (r *Registry) Register(cmd *Command) {
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.commands[alias] = cmd
	}
}

func (r *Registry) Get(name string) *Command {
	return r.commands[name]
}

func (r *Registry) List() []*Command {
	var list []*Command
	seen := make(map[string]bool)
	for _, cmd := range r.commands {
		if !seen[cmd.Name] {
			seen[cmd.Name] = true
			list = append(list, cmd)
		}
	}
	return list
}

func (r *Registry) Execute(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	parts := strings.SplitN(input, " ", 2)
	name := strings.TrimPrefix(parts[0], "/")
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}

	cmd := r.Get(name)
	if cmd == nil {
		return "", fmt.Errorf("unknown command: /%s", name)
	}
	return cmd.Handler(ctx, args)
}
