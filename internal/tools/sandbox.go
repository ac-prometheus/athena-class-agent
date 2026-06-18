//go:build linux

package tools

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
)

// SandboxMode controls how commands are executed.
type SandboxMode string

const (
	SandboxModeContainer  SandboxMode = "container"
	SandboxModeUser       SandboxMode = "user"
	SandboxModePermissive SandboxMode = "permissive"
	SandboxModeNone       SandboxMode = "none"
)

// SandboxConfig is the configuration for a Sandbox instance.
type SandboxConfig struct {
	Mode         SandboxMode
	AllowedPaths []string
	BlockedCmds  []string
	User         string // unix username for user mode
}

// Sandbox executes shell commands subject to the configured policy.
type Sandbox struct {
	cfg SandboxConfig
}

// NewSandbox returns a Sandbox configured with cfg.
func NewSandbox(cfg SandboxConfig) *Sandbox {
	return &Sandbox{cfg: cfg}
}

// Execute runs command in the sandbox. It checks the blocklist first,
// then dispatches to the appropriate exec method for the configured mode.
func (s *Sandbox) Execute(ctx context.Context, command string) (string, error) {
	if err := s.checkBlocklist(command); err != nil {
		return "", err
	}
	switch s.cfg.Mode {
	case SandboxModeContainer:
		return s.execInContainer(ctx, command)
	case SandboxModeUser:
		return s.execAsUser(ctx, command)
	case SandboxModePermissive:
		return s.execPermissive(ctx, command)
	default:
		return s.execDirect(ctx, command)
	}
}

// checkBlocklist rejects commands whose first token matches any blocked command.
func (s *Sandbox) checkBlocklist(command string) error {
	if len(s.cfg.BlockedCmds) == 0 {
		return nil
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil
	}
	first := fields[0]
	// Strip path prefix for comparison.
	if idx := strings.LastIndex(first, "/"); idx >= 0 {
		first = first[idx+1:]
	}
	for _, blocked := range s.cfg.BlockedCmds {
		if first == blocked {
			return fmt.Errorf("sandbox: command %q is blocked by policy (blocked command: %s)", first, blocked)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Sandbox) execDirect(ctx context.Context, command string) (string, error) {
	slog.Debug("sandbox exec", "mode", "none", "cmd", truncate(command, 80))
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec: %w", err)
	}
	return string(out), nil
}

func (s *Sandbox) execPermissive(ctx context.Context, command string) (string, error) {
	// Path allowlist validation is a stub — AllowedPaths enforcement planned for a future pass.
	if len(s.cfg.AllowedPaths) > 0 {
		slog.Warn("sandbox: permissive mode AllowedPaths enforcement not yet implemented; executing anyway")
	}
	slog.Debug("sandbox exec", "mode", "permissive", "cmd", truncate(command, 80))
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec: %w", err)
	}
	return string(out), nil
}

func (s *Sandbox) execAsUser(ctx context.Context, command string) (string, error) {
	slog.Debug("sandbox exec", "mode", "user", "user", s.cfg.User, "cmd", truncate(command, 80))
	u, err := user.Lookup(s.cfg.User)
	if err != nil {
		return "", fmt.Errorf("sandbox: lookup user %q: %w", s.cfg.User, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return "", fmt.Errorf("sandbox: parse uid %q: %w", u.Uid, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return "", fmt.Errorf("sandbox: parse gid %q: %w", u.Gid, err)
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec as user %s: %w", s.cfg.User, err)
	}
	return string(out), nil
}

func (s *Sandbox) execInContainer(ctx context.Context, command string) (string, error) {
	slog.Debug("sandbox exec", "mode", "container", "cmd", truncate(command, 80))
	containerName := os.Getenv("SANDBOX_CONTAINER_NAME")
	if containerName == "" {
		return "", fmt.Errorf("sandbox: SANDBOX_CONTAINER_NAME not set for container mode")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker exec: %w", err)
	}
	return string(out), nil
}
