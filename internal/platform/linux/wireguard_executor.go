package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MyHeartRaces/BigRedButton/internal/planner"
	"github.com/MyHeartRaces/BigRedButton/internal/wireguard"
)

var ErrUnsupportedWireGuardStep = errors.New("unsupported linux wireguard executor step")

type WireGuardConfigWriter interface {
	WriteConfig(ctx context.Context, rendered string) (path string, cleanup func(context.Context) error, err error)
}

type FileWireGuardConfigWriter struct {
	RuntimeRoot string
}

func (w FileWireGuardConfigWriter) WriteConfig(ctx context.Context, rendered string) (string, func(context.Context) error, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	root := strings.TrimSpace(w.RuntimeRoot)
	if root == "" {
		return "", nil, fmt.Errorf("wireguard config runtime root is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", nil, fmt.Errorf("create wireguard config directory: %w", err)
	}
	path := filepath.Join(root, "wg-setconf.conf")
	if err := os.WriteFile(path, []byte(rendered), 0o600); err != nil {
		return "", nil, fmt.Errorf("write wireguard config: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", nil, fmt.Errorf("set wireguard config permissions: %w", err)
	}
	cleanup := func(ctx context.Context) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove wireguard config: %w", err)
		}
		return nil
	}
	return path, cleanup, nil
}

type WireGuardExecutor struct {
	config     wireguard.Config
	runner     CommandRunner
	writer     WireGuardConfigWriter
	operations []Operation
}

type WireGuardExecutorOptions struct {
	Config       wireguard.Config
	Runner       CommandRunner
	ConfigWriter WireGuardConfigWriter
	RuntimeRoot  string
}

func NewWireGuardExecutor(options WireGuardExecutorOptions) (*WireGuardExecutor, error) {
	if err := options.Config.Validate(); err != nil {
		return nil, err
	}
	runner := options.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	writer := options.ConfigWriter
	if writer == nil {
		writer = FileWireGuardConfigWriter{RuntimeRoot: options.RuntimeRoot}
	}
	return &WireGuardExecutor{
		config: options.Config,
		runner: runner,
		writer: writer,
	}, nil
}

func (e *WireGuardExecutor) Apply(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("wireguard executor is nil")
	}

	switch step.ID {
	case "create-wireguard-interface":
		command, err := WireGuardCreateInterfaceCommand(e.config.InterfaceName)
		if err != nil {
			return err
		}
		return e.run(ctx, OperationApply, step.ID, command)
	case "apply-wireguard-addresses":
		for _, address := range e.config.Addresses {
			command, err := WireGuardAddAddressCommand(e.config.InterfaceName, address)
			if err != nil {
				return err
			}
			if err := e.run(ctx, OperationApply, step.ID, command); err != nil {
				return err
			}
		}
		command, err := WireGuardSetMTUCommand(e.config.InterfaceName, e.config.MTU)
		if err != nil {
			return err
		}
		if err := e.run(ctx, OperationApply, step.ID, command); err != nil {
			return err
		}
		command, err = WireGuardSetUpCommand(e.config.InterfaceName)
		if err != nil {
			return err
		}
		return e.run(ctx, OperationApply, step.ID, command)
	case "apply-wireguard-peer":
		return e.applyPeer(ctx, step)
	case "apply-client-routes":
		for _, allowedIP := range e.config.AllowedIPs {
			command, err := WireGuardRouteReplaceCommand(e.config.InterfaceName, allowedIP)
			if err != nil {
				return err
			}
			if err := e.run(ctx, OperationApply, step.ID, command); err != nil {
				return err
			}
		}
		return nil
	case "remove-client-routes":
		for _, allowedIP := range e.config.AllowedIPs {
			command, err := WireGuardRouteDeleteCommand(e.config.InterfaceName, allowedIP)
			if err != nil {
				return err
			}
			if err := e.run(ctx, OperationApply, step.ID, command); err != nil {
				return err
			}
		}
		return nil
	case "remove-wireguard-interface":
		command, err := WireGuardDeleteInterfaceCommand(e.config.InterfaceName)
		if err != nil {
			return err
		}
		return e.run(ctx, OperationApply, step.ID, command)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedWireGuardStep, step.ID)
	}
}

func (e *WireGuardExecutor) Rollback(ctx context.Context, step planner.Step) error {
	if e == nil {
		return fmt.Errorf("wireguard executor is nil")
	}

	switch step.ID {
	case "create-wireguard-interface", "apply-wireguard-addresses", "apply-wireguard-peer":
		command, err := WireGuardDeleteInterfaceCommand(e.config.InterfaceName)
		if err != nil {
			return err
		}
		return e.run(ctx, OperationRollback, step.ID, command)
	case "apply-client-routes":
		for _, allowedIP := range e.config.AllowedIPs {
			command, err := WireGuardRouteDeleteCommand(e.config.InterfaceName, allowedIP)
			if err != nil {
				return err
			}
			if err := e.run(ctx, OperationRollback, step.ID, command); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedWireGuardStep, step.ID)
	}
}

func (e *WireGuardExecutor) Operations() []Operation {
	if e == nil {
		return nil
	}
	operations := make([]Operation, len(e.operations))
	copy(operations, e.operations)
	return operations
}

func (e *WireGuardExecutor) applyPeer(ctx context.Context, step planner.Step) error {
	rendered, err := wireguard.RenderSetConf(e.config)
	if err != nil {
		return err
	}
	path, cleanup, err := e.writer.WriteConfig(ctx, rendered)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer func() {
			_ = cleanup(context.Background())
		}()
	}
	command, err := WireGuardSetConfigCommand(e.config.InterfaceName, path)
	if err != nil {
		return err
	}
	return e.run(ctx, OperationApply, step.ID, command)
}

func (e *WireGuardExecutor) run(ctx context.Context, phase OperationPhase, stepID string, command Command) error {
	e.operations = append(e.operations, Operation{
		Phase:   phase,
		StepID:  stepID,
		Command: &command,
	})
	output, err := e.runner.Run(ctx, command)
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("run %s: %w", command.String(), err)
		}
		return fmt.Errorf("run %s: %w: %s", command.String(), err, detail)
	}
	return nil
}
