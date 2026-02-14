package providers

import "fmt"

// ModelInfo contains metadata for each supported model.
type ModelInfo struct {
	ID                string
	ProviderType      string // "anthropic" | "openai_compat"
	BaseURL           string
	MaxContext         int
	InputCostPerMTok  float64 // USD per million input tokens
	OutputCostPerMTok float64 // USD per million output tokens
	ExtraParams       map[string]any
}

// SupportedModels is the definitive list of models Argus supports.
// Users choose from this list.
var SupportedModels = map[string]ModelInfo{
	"claude-opus-4-6": {
		ID:                "claude-opus-4-6",
		ProviderType:      "anthropic",
		BaseURL:           "https://api.anthropic.com",
		MaxContext:         200000,
		InputCostPerMTok:  5.0,
		OutputCostPerMTok: 25.0,
	},
	"gpt-5.2": {
		ID:                "gpt-5.2",
		ProviderType:      "openai_compat",
		BaseURL:           "https://api.openai.com/v1",
		MaxContext:         128000,
		InputCostPerMTok:  10.0,
		OutputCostPerMTok: 30.0,
	},
	"glm-5": {
		ID:                "glm-5",
		ProviderType:      "openai_compat",
		BaseURL:           "https://api.z.ai/api/paas/v4/",
		MaxContext:         128000,
		InputCostPerMTok:  0.50,
		OutputCostPerMTok: 2.0,
	},
	"kimi-k2.5": {
		ID:                "kimi-k2.5",
		ProviderType:      "openai_compat",
		BaseURL:           "https://api.moonshot.ai/v1",
		MaxContext:         256000,
		InputCostPerMTok:  0.60,
		OutputCostPerMTok: 3.0,
		ExtraParams: map[string]any{
			"thinking": map[string]string{"type": "disabled"},
		},
	},
	"minimax-m2.5": {
		ID:                "minimax-m2.5",
		ProviderType:      "openai_compat",
		BaseURL:           "https://api.minimax.chat/v1",
		MaxContext:         1000000,
		InputCostPerMTok:  0.15,
		OutputCostPerMTok: 1.20,
	},
}

// apiKeyMapping maps model IDs to the key name used in the apiKeys map.
var apiKeyMapping = map[string]string{
	"claude-opus-4-6": "anthropic",
	"gpt-5.2":         "openai",
	"glm-5":           "glm",
	"kimi-k2.5":       "kimi",
	"minimax-m2.5":    "minimax",
}

// NewProvider creates the correct Provider for the given model ID and API keys.
// Returns error if the model is not in SupportedModels or the required API key is missing.
func NewProvider(modelID string, apiKeys map[string]string) (Provider, error) {
	model, ok := SupportedModels[modelID]
	if !ok {
		return nil, fmt.Errorf("providers: unknown model %q", modelID)
	}

	keyName, ok := apiKeyMapping[modelID]
	if !ok {
		return nil, fmt.Errorf("providers: no API key mapping for model %q", modelID)
	}

	apiKey := apiKeys[keyName]
	if apiKey == "" {
		return nil, fmt.Errorf("providers: API key %q is required for model %q", keyName, modelID)
	}

	switch model.ProviderType {
	case "anthropic":
		return NewAnthropicProvider(apiKey, model.ID), nil
	case "openai_compat":
		return NewOpenAICompatProvider(apiKey, model.ID, model.BaseURL, model.ExtraParams), nil
	default:
		return nil, fmt.Errorf("providers: unknown provider type %q for model %q", model.ProviderType, modelID)
	}
}

// ModelIDs returns a sorted list of all supported model IDs.
func ModelIDs() []string {
	// Return in a stable, meaningful order
	return []string{
		"claude-opus-4-6",
		"gpt-5.2",
		"glm-5",
		"kimi-k2.5",
		"minimax-m2.5",
	}
}
