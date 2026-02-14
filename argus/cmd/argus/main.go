package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/argus-beta/argus/internal/config"
	"github.com/argus-beta/argus/internal/core/agent"
	"github.com/argus-beta/argus/internal/core/tools"
	"github.com/argus-beta/argus/internal/memory"
	"github.com/argus-beta/argus/internal/providers"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "argus",
		Short: "Argus — AI-powered security intelligence platform",
		Long:  "Argus runs AI agents to find security vulnerabilities in your codebase.",
	}

	root.AddCommand(
		initCmd(),
		configCmd(),
		modelsCmd(),
		projectCmd(),
		sessionCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- argus init ---

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := &config.Config{
				Defaults: config.Defaults{
					Model: "claude-opus-4-6",
					Mode:  "single",
				},
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			home, _ := os.UserHomeDir()
			fmt.Printf("Config created at %s\n", filepath.Join(home, ".config", "argus", "config.toml"))
			fmt.Println("Edit the file to add your API keys, then run: argus config set-key <provider> <key>")
			return nil
		},
	}
}

// --- argus config ---

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	setKey := &cobra.Command{
		Use:   "set-key <provider> <key>",
		Short: "Set an API key (providers: anthropic, openai, glm, kimi, minimax)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			provider, key := args[0], args[1]

			cfg, err := config.Load()
			if err != nil {
				// Create new config if it doesn't exist
				cfg = &config.Config{
					Defaults: config.Defaults{Model: "claude-opus-4-6", Mode: "single"},
				}
			}

			switch provider {
			case "anthropic":
				cfg.Keys.Anthropic = key
			case "openai":
				cfg.Keys.OpenAI = key
			case "glm":
				cfg.Keys.GLM = key
			case "kimi":
				cfg.Keys.Kimi = key
			case "minimax":
				cfg.Keys.MiniMax = key
			default:
				return fmt.Errorf("unknown provider %q (use: anthropic, openai, glm, kimi, minimax)", provider)
			}

			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Printf("API key for %s saved.\n", provider)
			return nil
		},
	}

	cmd.AddCommand(setKey)
	return cmd
}

// --- argus models ---

func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List supported models with pricing",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Supported Models:")
			fmt.Println()
			for _, id := range providers.ModelIDs() {
				m := providers.SupportedModels[id]
				fmt.Printf("  %-20s %-15s ctx:%dk  in:$%.2f/MTok  out:$%.2f/MTok\n",
					m.ID, m.ProviderType, m.MaxContext/1000,
					m.InputCostPerMTok, m.OutputCostPerMTok)
			}
		},
	}
}

// --- argus project ---

func projectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage projects",
	}

	addCmd := &cobra.Command{
		Use:   "add <name> <path>",
		Short: "Register a project for analysis",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, rootPath := args[0], args[1]

			absPath, err := filepath.Abs(rootPath)
			if err != nil {
				return fmt.Errorf("invalid path: %w", err)
			}

			info, err := os.Stat(absPath)
			if err != nil || !info.IsDir() {
				return fmt.Errorf("path %q does not exist or is not a directory", absPath)
			}

			dbPath, err := memory.DefaultDBPath()
			if err != nil {
				return err
			}
			store, err := memory.NewStore(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			project := &memory.Project{
				Name:     name,
				RootPath: absPath,
				Config:   "{}",
			}

			if err := store.CreateProject(context.Background(), project); err != nil {
				return fmt.Errorf("failed to add project: %w", err)
			}

			fmt.Printf("Project %q registered (id: %s, path: %s)\n", name, project.ID, absPath)
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List registered projects",
		RunE: func(cmd *cobra.Command, args []string) error {
			dbPath, err := memory.DefaultDBPath()
			if err != nil {
				return err
			}
			store, err := memory.NewStore(dbPath)
			if err != nil {
				return err
			}
			defer store.Close()

			projects, err := store.ListProjects(context.Background())
			if err != nil {
				return err
			}

			if len(projects) == 0 {
				fmt.Println("No projects registered. Use: argus project add <name> <path>")
				return nil
			}

			for _, p := range projects {
				fmt.Printf("  %-20s %s  (id: %s)\n", p.Name, p.RootPath, p.ID[:8])
			}
			return nil
		},
	}

	cmd.AddCommand(addCmd, listCmd)
	return cmd
}

// --- argus session ---

func sessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage investigation sessions",
	}

	// session start
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start a new investigation session",
		RunE:  runSessionStart,
	}

	startCmd.Flags().String("project", "", "Project name or path (required)")
	startCmd.Flags().String("model", "", "Model ID (default from config)")
	startCmd.Flags().String("mode", "single", "Mode: single or collab")
	startCmd.Flags().String("model-b", "", "Model for agent B (collab mode only)")
	startCmd.Flags().String("focus", "", "Investigation focus")
	startCmd.Flags().String("focus-b", "", "Agent B focus (collab mode only)")
	startCmd.Flags().Float64("max-cost", 2.0, "Maximum cost in USD")
	startCmd.Flags().Int("max-tokens", 500000, "Maximum total tokens")
	startCmd.Flags().Bool("verbose", false, "Show tool calls in real time")
	_ = startCmd.MarkFlagRequired("project")

	// session findings
	findingsCmd := &cobra.Command{
		Use:   "findings <project>",
		Short: "List findings for a project",
		Args:  cobra.ExactArgs(1),
		RunE:  runSessionFindings,
	}

	cmd.AddCommand(startCmd, findingsCmd)
	return cmd
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	modelFlag, _ := cmd.Flags().GetString("model")
	modeFlag, _ := cmd.Flags().GetString("mode")
	modelBFlag, _ := cmd.Flags().GetString("model-b")
	focusFlag, _ := cmd.Flags().GetString("focus")
	focusBFlag, _ := cmd.Flags().GetString("focus-b")
	maxCost, _ := cmd.Flags().GetFloat64("max-cost")
	maxTokens, _ := cmd.Flags().GetInt("max-tokens")
	verbose, _ := cmd.Flags().GetBool("verbose")

	// Load config
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w\nRun 'argus init' to create a config file", err)
	}

	// Determine model
	if modelFlag == "" {
		modelFlag = cfg.Defaults.Model
	}
	if modelFlag == "" {
		modelFlag = "claude-opus-4-6"
	}

	// Validate model and API key
	if err := cfg.ValidateForModel(modelFlag); err != nil {
		return err
	}

	// Open store
	dbPath, err := memory.DefaultDBPath()
	if err != nil {
		return err
	}
	store, err := memory.NewStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	// Find project
	project, err := resolveProject(store, projectFlag)
	if err != nil {
		return err
	}

	// Create tool executor
	logger := tools.NewToolLogger(verbose)
	executor, err := tools.NewToolExecutor(project.RootPath, logger)
	if err != nil {
		return fmt.Errorf("failed to create tool executor: %w", err)
	}

	// Build budget
	budget := agent.Budget{
		MaxTokens:    maxTokens,
		MaxCostUSD:   maxCost,
		MaxToolCalls: 200,
		MaxDuration:  agent.DefaultBudget().MaxDuration,
		MaxTurns:     50,
	}

	// Set up context with Ctrl+C handling
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	sessionID := uuid.New().String()

	if modeFlag == "collab" {
		return runCollabSession(ctx, store, project, executor, cfg, sessionID, modelFlag, modelBFlag, focusFlag, focusBFlag, budget, verbose)
	}
	return runSingleSession(ctx, store, project, executor, cfg, sessionID, modelFlag, focusFlag, budget, verbose)
}

func runSingleSession(ctx context.Context, store memory.Store, project *memory.Project, executor *tools.ToolExecutor,
	cfg *config.Config, sessionID, modelID, focus string, budget agent.Budget, verbose bool) error {

	// Create session record
	session := &memory.Session{
		ID:        sessionID,
		ProjectID: project.ID,
		ModelA:    modelID,
		Mode:      "single",
		Status:    "running",
	}
	if err := store.CreateSession(ctx, session); err != nil {
		return err
	}

	// Create provider
	provider, err := providers.NewProvider(modelID, cfg.ToAPIKeysMap())
	if err != nil {
		return err
	}

	// Load context from store
	findings, _ := store.ListFindings(ctx, project.ID)
	prevTitles := make([]string, len(findings))
	for i, f := range findings {
		prevTitles[i] = f.Title
	}
	areas, _ := store.GetInvestigatedAreas(ctx, project.ID)
	areaStrings := make([]string, len(areas))
	for i, a := range areas {
		areaStrings[i] = fmt.Sprintf("%s: %s", a.Path, a.Pattern)
	}

	promptCfg := agent.PromptConfig{
		Mode:              "single",
		ProjectName:       project.Name,
		RootPath:          project.RootPath,
		Focus:             focus,
		PreviousFindings:  prevTitles,
		InvestigatedAreas: areaStrings,
	}

	runner := agent.NewRunner(agent.RunnerConfig{
		SessionID: sessionID,
		ProjectID: project.ID,
		AgentName: "single",
		Provider:  provider,
		Executor:  executor,
		Store:     store,
		Prompt:    promptCfg,
		Budget:    budget,
		Verbose:   verbose,
	})

	fmt.Printf("Starting session %s...\n", sessionID[:8])
	fmt.Printf("Model: %s | Project: %s | Budget: $%.2f\n\n", modelID, project.Name, budget.MaxCostUSD)

	events := runner.Run(ctx)
	return displayEvents(events, verbose)
}

func runCollabSession(ctx context.Context, store memory.Store, project *memory.Project, executor *tools.ToolExecutor,
	cfg *config.Config, sessionID, modelA, modelB, focusA, focusB string, budget agent.Budget, verbose bool) error {

	if modelB == "" {
		return fmt.Errorf("--model-b is required for collaborative mode")
	}
	if err := cfg.ValidateForModel(modelB); err != nil {
		return err
	}

	session := &memory.Session{
		ID:        sessionID,
		ProjectID: project.ID,
		ModelA:    modelA,
		ModelB:    modelB,
		Mode:      "collaborative",
		Status:    "running",
	}
	if err := store.CreateSession(ctx, session); err != nil {
		return err
	}

	// Load context
	findings, _ := store.ListFindings(ctx, project.ID)
	prevTitles := make([]string, len(findings))
	for i, f := range findings {
		prevTitles[i] = f.Title
	}
	areas, _ := store.GetInvestigatedAreas(ctx, project.ID)
	areaStrings := make([]string, len(areas))
	for i, a := range areas {
		areaStrings[i] = fmt.Sprintf("%s: %s", a.Path, a.Pattern)
	}

	collabCfg := agent.CollabConfig{
		SessionID:         sessionID,
		ProjectID:         project.ID,
		ModelA:            modelA,
		ModelB:            modelB,
		FocusA:            focusA,
		FocusB:            focusB,
		APIKeys:           cfg.ToAPIKeysMap(),
		Executor:          executor,
		Store:             store,
		Budget:            budget,
		Verbose:           verbose,
		ProjectName:       project.Name,
		RootPath:          project.RootPath,
		PreviousFindings:  prevTitles,
		InvestigatedAreas: areaStrings,
	}

	fmt.Printf("Starting collaborative session %s...\n", sessionID[:8])
	fmt.Printf("Agent A: %s | Agent B: %s | Project: %s\n\n", modelA, modelB, project.Name)

	events, err := agent.RunCollaborative(ctx, collabCfg)
	if err != nil {
		return err
	}

	return displayEvents(events, verbose)
}

func displayEvents(events <-chan agent.RunEvent, verbose bool) error {
	for evt := range events {
		prefix := ""
		if evt.AgentName == "agent_a" {
			prefix = "[A] "
		} else if evt.AgentName == "agent_b" {
			prefix = "[B] "
		}

		switch evt.Type {
		case agent.EventText:
			fmt.Print(evt.Text)

		case agent.EventToolCall:
			if verbose {
				fmt.Fprintf(os.Stderr, "\n%s→ %s\n", prefix, evt.ToolName)
			}

		case agent.EventToolResult:
			if verbose {
				status := "✓"
				if evt.IsError {
					status = "✗"
				}
				fmt.Fprintf(os.Stderr, "%s  %s %s\n", prefix, status, evt.Text)
			}

		case agent.EventFindingRecorded:
			fmt.Fprintf(os.Stderr, "\n%sFINDING: %s\n", prefix, evt.Text)

		case agent.EventDone:
			fmt.Fprintf(os.Stderr, "\n\n%sSession complete. %s\n", prefix, evt.Text)

		case agent.EventBudgetExceeded:
			fmt.Fprintf(os.Stderr, "\n\n%sBudget exceeded: %s\n", prefix, evt.Text)

		case agent.EventError:
			fmt.Fprintf(os.Stderr, "\n%sError: %s\n", prefix, evt.Text)
		}
	}
	return nil
}

func runSessionFindings(cmd *cobra.Command, args []string) error {
	projectFlag := args[0]

	dbPath, err := memory.DefaultDBPath()
	if err != nil {
		return err
	}
	store, err := memory.NewStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	project, err := resolveProject(store, projectFlag)
	if err != nil {
		return err
	}

	findings, err := store.ListFindings(context.Background(), project.ID)
	if err != nil {
		return err
	}

	if len(findings) == 0 {
		fmt.Printf("No findings for project %q.\n", project.Name)
		return nil
	}

	fmt.Printf("Findings for %q (%d total):\n\n", project.Name, len(findings))
	for i, f := range findings {
		fmt.Printf("%d. [%s] %s\n", i+1, f.Severity, f.Title)
		fmt.Printf("   Location:   %s\n", f.Location)
		fmt.Printf("   Confidence: %s\n", f.Confidence)
		if f.Category != "" {
			fmt.Printf("   Category:   %s\n", f.Category)
		}
		fmt.Printf("   %s\n\n", f.Description)
	}
	return nil
}

// resolveProject finds a project by name or path.
func resolveProject(store memory.Store, nameOrPath string) (*memory.Project, error) {
	ctx := context.Background()

	// Try by path first
	absPath, err := filepath.Abs(nameOrPath)
	if err == nil {
		if p, err := store.GetProjectByPath(ctx, absPath); err == nil {
			return p, nil
		}
	}

	// Try listing and matching by name
	projects, err := store.ListProjects(ctx)
	if err != nil {
		return nil, err
	}

	for _, p := range projects {
		if p.Name == nameOrPath {
			return p, nil
		}
	}

	return nil, fmt.Errorf("project %q not found. Run 'argus project add <name> <path>' first", nameOrPath)
}
