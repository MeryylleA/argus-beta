package tools

import (
	"context"
	"fmt"
)

// Parameter extraction helpers.
// LLM tool calls send parameters as map[string]any (from JSON parsing).
// These helpers provide safe type extraction with clear error messages
// that help the agent self-correct when it passes wrong parameter types.

// extractString gets a string parameter, returning an error if missing or wrong type.
func extractString(params map[string]any, key string, required bool) (string, error) {
	val, exists := params[key]
	if !exists {
		if required {
			return "", fmt.Errorf("missing required parameter: %q", key)
		}
		return "", nil
	}

	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %q must be a string, got %T", key, val)
	}
	return str, nil
}

// extractInt gets an integer parameter. Handles both int and float64
// (JSON numbers are parsed as float64 by encoding/json).
func extractInt(params map[string]any, key string, required bool, defaultVal int) (int, error) {
	val, exists := params[key]
	if !exists {
		if required {
			return 0, fmt.Errorf("missing required parameter: %q", key)
		}
		return defaultVal, nil
	}

	switch v := val.(type) {
		case int:
			return v, nil
		case int64:
			return int(v), nil
		case float64:
			// JSON numbers come as float64 — this is the common case
			return int(v), nil
		default:
			return 0, fmt.Errorf("parameter %q must be an integer, got %T", key, val)
	}
}

// extractContext retrieves the context injected by the executor.
// Falls back to context.Background() if not present (for direct tool testing).
func extractContext(params map[string]any) context.Context {
	if ctx, ok := params["__ctx"].(context.Context); ok {
		return ctx
	}
	return context.Background()
}
