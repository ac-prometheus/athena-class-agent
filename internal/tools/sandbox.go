//go:build linux

package tools

import (
	"context"
	"fmt"
	"log/slog"
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
	Mode          SandboxMode
	AllowedPaths  []string
	BlockedCmds   []string
	User          string // unix username for user mode
	ContainerName string // read once at startup for container mode
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
	case SandboxModeNone:
		return s.execDirect(ctx, command)
	case SandboxModePermissive:
		return s.execPermissive(ctx, command)
	case SandboxModeUser:
		return s.execAsUser(ctx, command)
	case SandboxModeContainer:
		return s.execInContainer(ctx, command)
	default:
		return "", fmt.Errorf("sandbox: unrecognized mode %q — refusing to execute", s.cfg.Mode)
	}
}

// checkBlocklist scans all tokens in the command against the blocked list.
// Advisory only — the real security boundary is the sandbox mode. But scanning
// all tokens prevents the trivial bypass of wrapping a blocked command in sh -c.
func (s *Sandbox) checkBlocklist(command string) error {
	if len(s.cfg.BlockedCmds) == 0 {
		return nil
	}
	fields := strings.Fields(command)
	for _, token := range fields {
		// Strip path prefix for comparison.
		bare := token
		if idx := strings.LastIndex(bare, "/"); idx >= 0 {
			bare = bare[idx+1:]
		}
		for _, blocked := range s.cfg.BlockedCmds {
			if bare == blocked {
				return fmt.Errorf("sandbox: %q matches blocked command %q — rejected (advisory blocklist; real boundary is sandbox mode %q)", token, blocked, s.cfg.Mode)
			}
		}
	}
	return nil
}

func truncateSandbox(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Sandbox) execDirect(ctx context.Context, command string) (string, error) {
	slog.Debug("sandbox exec", "mode", "none", "cmd", truncateSandbox(command, 80))
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec: %w", err)
	}
	return string(out), nil
}

func (s *Sandbox) execPermissive(ctx context.Context, command string) (string, error) {
	if len(s.cfg.AllowedPaths) > 0 {
		return "", fmt.Errorf("sandbox: permissive mode AllowedPaths enforcement not yet implemented — refusing to execute (fail closed)")
	}
	slog.Debug("sandbox exec", "mode", "permissive", "cmd", truncateSandbox(command, 80))
	out, err := exec.CommandContext(ctx, "sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("exec: %w", err)
	}
	return string(out), nil
}

func (s *Sandbox) execAsUser(ctx context.Context, command string) (string, error) {
	slog.Debug("sandbox exec", "mode", "user", "user", s.cfg.User, "cmd", truncateSandbox(command, 80))
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
	slog.Debug("sandbox exec", "mode", "container", "cmd", truncateSandbox(command, 80))
	if s.cfg.ContainerName == "" {
		return "", fmt.Errorf("sandbox: container mode requires ContainerName in config")
	}
	cmd := exec.CommandContext(ctx, "docker", "exec", s.cfg.ContainerName, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker exec: %w", err)
	}
	return string(out), nil
}
