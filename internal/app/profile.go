// Package app provides the validated composition layer between the executable
// and the domain packages. It owns runtime profiles, dependency validation,
// and the session lifecycle service.
package app

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/ac-prometheus/athena-class-agent/internal/platform"
	"github.com/ac-prometheus/athena-class-agent/pkg"
)

// RuntimeProfile declares which capabilities the executable intends to use.
// Each profile defines required and optional dependencies; startup fails when
// a required dependency is absent rather than silently degrading.
type RuntimeProfile string

const (
	// ProfileErsaProduction is the full cognitive lifecycle: all stores,
	// memory, Aegis, tools, and LLM are required. Nothing may be nil.
	ProfileErsaProduction RuntimeProfile = "ersa_production"

	// ProfileDevelopment is the full lifecycle with relaxed validation.
	// Stores and LLM are required; Aegis, tools, and channels are optional
	// (logged as warnings when absent).
	ProfileDevelopment RuntimeProfile = "development"

	// ProfileConnectivityTest verifies DB and LLM connectivity only.
	// No session is run.
	ProfileConnectivityTest RuntimeProfile = "connectivity_test"
)

// ValidProfiles enumerates accepted profile strings.
var ValidProfiles = map[RuntimeProfile]bool{
	ProfileErsaProduction:   true,
	ProfileDevelopment:      true,
	ProfileConnectivityTest: true,
}

// ParseProfile converts a string to a RuntimeProfile, returning an error
// for unrecognized values. Empty string defaults to ProfileDevelopment.
func ParseProfile(s string) (RuntimeProfile, error) {
	if s == "" {
		return ProfileDevelopment, nil
	}
	p := RuntimeProfile(s)
	if !ValidProfiles[p] {
		names := make([]string, 0, len(ValidProfiles))
		for k := range ValidProfiles {
			names = append(names, string(k))
		}
		return "", fmt.Errorf("unknown runtime profile %q (valid: %s)", s, strings.Join(names, ", "))
	}
	return p, nil
}

// Dependencies holds every typed dependency the application needs.
// Validate checks the dependency set against a RuntimeProfile.
type Dependencies struct {
	// Infrastructure
	DB     platform.DB
	Config *platform.Config
	LLM    pkg.LLMClient

	// Repository ports (Sprint 3E)
	JobStore           pkg.MetabolismJobStore
	ConsolidationStore pkg.ConsolidationStore
	LifecycleStore     pkg.LifecycleStore
	AssemblyStore      pkg.AssemblyStore

	// Domain services
	MemoryStore       pkg.MemoryStore
	EmbeddingProvider pkg.EmbeddingProvider
	Gateway           pkg.ContentGateway // Aegis
	ToolRegistry      pkg.ToolRegistry
}

// Validate checks that all dependencies required by the given profile are present.
// Returns an error listing every missing requirement (not just the first).
// Optional-but-absent dependencies are logged as warnings.
func (d *Dependencies) Validate(profile RuntimeProfile) error {
	var missing []string
	var warnings []string

	check := func(name string, present bool, required bool) {
		if !present {
			if required {
				missing = append(missing, name)
			} else {
				warnings = append(warnings, name)
			}
		}
	}

	switch profile {
	case ProfileErsaProduction:
		check("DB", d.DB != nil, true)
		check("LLM", d.LLM != nil, true)
		check("Config", d.Config != nil, true)
		check("JobStore", d.JobStore != nil, true)
		check("ConsolidationStore", d.ConsolidationStore != nil, true)
		check("LifecycleStore", d.LifecycleStore != nil, true)
		check("AssemblyStore", d.AssemblyStore != nil, true)
		check("MemoryStore", d.MemoryStore != nil, true)
		check("EmbeddingProvider", d.EmbeddingProvider != nil, true)
		check("Gateway (Aegis)", d.Gateway != nil, true)
		check("ToolRegistry", d.ToolRegistry != nil, true)

	case ProfileDevelopment:
		check("DB", d.DB != nil, true)
		check("LLM", d.LLM != nil, true)
		check("Config", d.Config != nil, true)
		check("JobStore", d.JobStore != nil, true)
		check("ConsolidationStore", d.ConsolidationStore != nil, true)
		check("LifecycleStore", d.LifecycleStore != nil, true)
		check("AssemblyStore", d.AssemblyStore != nil, true)
		check("MemoryStore", d.MemoryStore != nil, false)
		check("EmbeddingProvider", d.EmbeddingProvider != nil, false)
		check("Gateway (Aegis)", d.Gateway != nil, false)
		check("ToolRegistry", d.ToolRegistry != nil, false)

	case ProfileConnectivityTest:
		check("DB", d.DB != nil, true)
		check("LLM", d.LLM != nil, true)
		check("Config", d.Config != nil, true)

	default:
		return fmt.Errorf("unknown runtime profile: %q", profile)
	}

	for _, w := range warnings {
		slog.Warn("profile: optional dependency absent", "profile", profile, "dependency", w)
	}

	if len(missing) > 0 {
		return fmt.Errorf("profile %q requires: %s", profile, strings.Join(missing, ", "))
	}
	return nil
}
