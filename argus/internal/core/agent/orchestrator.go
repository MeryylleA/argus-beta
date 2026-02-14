package agent

import (
	"context"
	"sync"

	"github.com/argus-beta/argus/internal/core/channel"
	"github.com/argus-beta/argus/internal/core/tools"
	"github.com/argus-beta/argus/internal/memory"
	"github.com/argus-beta/argus/internal/providers"
)

// CollabConfig configures a collaborative investigation session.
type CollabConfig struct {
	SessionID string
	ProjectID string
	ModelA    string // model ID for agent A
	ModelB    string // model ID for agent B
	FocusA    string // e.g. "authentication and authorization patterns"
	FocusB    string // e.g. "input validation and injection vectors"
	APIKeys   map[string]string
	Executor  *tools.ToolExecutor
	Store     memory.Store
	Budget    Budget
	Verbose   bool

	// Prompt base fields
	ProjectName       string
	RootPath          string
	Scope             ScopeConfig
	BountyProgram     *BountyProgram
	PreviousFindings  []string
	InvestigatedAreas []string
}

// RunCollaborative starts both agents in parallel goroutines.
// Returns a merged channel of RunEvents from both agents.
// Each event includes AgentName so the caller can distinguish sources.
// The channel is closed when both agents finish.
func RunCollaborative(ctx context.Context, cfg CollabConfig) (<-chan RunEvent, error) {
	// Create providers for each agent
	providerA, err := providers.NewProvider(cfg.ModelA, cfg.APIKeys)
	if err != nil {
		return nil, err
	}
	providerB, err := providers.NewProvider(cfg.ModelB, cfg.APIKeys)
	if err != nil {
		return nil, err
	}

	// Shared channel for inter-agent communication
	ch := channel.New(cfg.Store, cfg.SessionID)

	// Build prompt configs
	promptA := PromptConfig{
		Mode:              "agent_a",
		ProjectName:       cfg.ProjectName,
		RootPath:          cfg.RootPath,
		Focus:             cfg.FocusA,
		Scope:             cfg.Scope,
		BountyProgram:     cfg.BountyProgram,
		PreviousFindings:  cfg.PreviousFindings,
		InvestigatedAreas: cfg.InvestigatedAreas,
		PartnerModel:      cfg.ModelB,
	}

	promptB := PromptConfig{
		Mode:              "agent_b",
		ProjectName:       cfg.ProjectName,
		RootPath:          cfg.RootPath,
		Focus:             cfg.FocusB,
		Scope:             cfg.Scope,
		BountyProgram:     cfg.BountyProgram,
		PreviousFindings:  cfg.PreviousFindings,
		InvestigatedAreas: cfg.InvestigatedAreas,
		PartnerModel:      cfg.ModelA,
	}

	// Create runners
	runnerA := NewRunner(RunnerConfig{
		SessionID: cfg.SessionID,
		ProjectID: cfg.ProjectID,
		AgentName: "agent_a",
		Provider:  providerA,
		Executor:  cfg.Executor,
		Store:     cfg.Store,
		Prompt:    promptA,
		Budget:    cfg.Budget,
		Channel:   ch,
		Verbose:   cfg.Verbose,
	})

	runnerB := NewRunner(RunnerConfig{
		SessionID: cfg.SessionID,
		ProjectID: cfg.ProjectID,
		AgentName: "agent_b",
		Provider:  providerB,
		Executor:  cfg.Executor,
		Store:     cfg.Store,
		Prompt:    promptB,
		Budget:    cfg.Budget,
		Channel:   ch,
		Verbose:   cfg.Verbose,
	})

	// Merge both event streams
	merged := make(chan RunEvent, 128)

	var wg sync.WaitGroup
	wg.Add(2)

	forward := func(src <-chan RunEvent) {
		defer wg.Done()
		for evt := range src {
			merged <- evt
		}
	}

	eventsA := runnerA.Run(ctx)
	eventsB := runnerB.Run(ctx)

	go forward(eventsA)
	go forward(eventsB)

	// Close merged channel when both agents finish
	go func() {
		wg.Wait()
		close(merged)
	}()

	return merged, nil
}
