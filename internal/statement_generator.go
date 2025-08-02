package internal

import (
	"context"
	"log/slog"
)

// StatementGenerator defines the interface for system-specific statement regeneration.
// Each system (transcript, feedback, etc.) implements this interface to handle its own
// regeneration logic, keeping the versioning system agnostic of generation details.
type StatementGenerator interface {
	// GenerateStatement takes a statement version and generates a new statement
	// using the system's specific logic and prompts. The version contains all the
	// metadata the system originally used to generate the statement.
	GenerateStatement(ctx context.Context, version *UserStatementVersion) (string, error)
}

// StatementRegeneratorRegistry manages the registration and lookup of statement regenerators.
type StatementGeneratorRegistry struct {
	regenerators map[string]StatementGenerator
	logger       *slog.Logger
}

// NewStatementGeneratorRegistry creates a new registry for statement regenerators.
func NewStatementGeneratorRegistry(logger *slog.Logger) *StatementGeneratorRegistry {
	return &StatementGeneratorRegistry{
		regenerators: make(map[string]StatementGenerator),
		logger:       logger,
	}
}

// Register adds a regenerator for a specific system type.
func (r *StatementGeneratorRegistry) Register(systemType string, regenerator StatementGenerator) {
	r.regenerators[systemType] = regenerator
	r.logger.Debug("registered statement regenerator", "systemType", systemType)
}

// GetRegenerator returns the regenerator for a system type, or nil if not found.
func (r *StatementGeneratorRegistry) GetRegenerator(systemType string) StatementGenerator {
	return r.regenerators[systemType]
}

// HasRegenerator checks if a regenerator exists for the given system type.
func (r *StatementGeneratorRegistry) HasRegenerator(systemType string) bool {
	_, exists := r.regenerators[systemType]
	return exists
}

// ListRegisteredSystems returns a list of all registered system types.
func (r *StatementGeneratorRegistry) ListRegisteredSystems() []string {
	systems := make([]string, 0, len(r.regenerators))
	for systemType := range r.regenerators {
		systems = append(systems, systemType)
	}
	return systems
}
