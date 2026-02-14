package providers

import "testing"

func TestSupportedModelsContainsAllModels(t *testing.T) {
	expectedIDs := []string{
		"claude-opus-4-6",
		"gpt-5.2",
		"glm-5",
		"kimi-k2.5",
		"minimax-m2.5",
	}

	for _, id := range expectedIDs {
		if _, ok := SupportedModels[id]; !ok {
			t.Errorf("SupportedModels missing model %q", id)
		}
	}

	if len(SupportedModels) != len(expectedIDs) {
		t.Errorf("SupportedModels has %d entries, expected %d", len(SupportedModels), len(expectedIDs))
	}
}

func TestNewProviderUnknownModel(t *testing.T) {
	_, err := NewProvider("nonexistent-model", map[string]string{})
	if err == nil {
		t.Fatal("expected error for unknown model")
	}
}

func TestNewProviderMissingAPIKey(t *testing.T) {
	_, err := NewProvider("claude-opus-4-6", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing API key")
	}
}

func TestNewProviderAnthropic(t *testing.T) {
	keys := map[string]string{"anthropic": "test-key-123"}
	p, err := NewProvider("claude-opus-4-6", keys)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if p.Name() != "anthropic" {
		t.Errorf("expected provider name 'anthropic', got %q", p.Name())
	}
	if p.ModelID() != "claude-opus-4-6" {
		t.Errorf("expected model ID 'claude-opus-4-6', got %q", p.ModelID())
	}
	if p.MaxContextTokens() != 200000 {
		t.Errorf("expected 200000 context tokens, got %d", p.MaxContextTokens())
	}
}

func TestNewProviderOpenAICompat(t *testing.T) {
	tests := []struct {
		modelID string
		keyName string
	}{
		{"gpt-5.2", "openai"},
		{"glm-5", "glm"},
		{"kimi-k2.5", "kimi"},
		{"minimax-m2.5", "minimax"},
	}

	for _, tt := range tests {
		keys := map[string]string{tt.keyName: "test-key"}
		p, err := NewProvider(tt.modelID, keys)
		if err != nil {
			t.Fatalf("%s: unexpected error: %s", tt.modelID, err)
		}
		if p.Name() != "openai_compat" {
			t.Errorf("%s: expected provider name 'openai_compat', got %q", tt.modelID, p.Name())
		}
		if p.ModelID() != tt.modelID {
			t.Errorf("expected model ID %q, got %q", tt.modelID, p.ModelID())
		}
	}
}

func TestModelIDsOrder(t *testing.T) {
	ids := ModelIDs()
	if len(ids) != 5 {
		t.Fatalf("expected 5 model IDs, got %d", len(ids))
	}
	if ids[0] != "claude-opus-4-6" {
		t.Errorf("expected first model to be 'claude-opus-4-6', got %q", ids[0])
	}
}

func TestKimiHasExtraParams(t *testing.T) {
	model := SupportedModels["kimi-k2.5"]
	if model.ExtraParams == nil {
		t.Fatal("kimi-k2.5 should have ExtraParams for thinking:disabled")
	}
	thinking, ok := model.ExtraParams["thinking"]
	if !ok {
		t.Fatal("kimi-k2.5 ExtraParams should contain 'thinking' key")
	}
	thinkingMap, ok := thinking.(map[string]string)
	if !ok {
		t.Fatal("thinking param should be map[string]string")
	}
	if thinkingMap["type"] != "disabled" {
		t.Errorf("thinking type should be 'disabled', got %q", thinkingMap["type"])
	}
}
