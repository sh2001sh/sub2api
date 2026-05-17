package service

import (
	"fmt"
	"strings"
)

// APIKeyModelQuotaState holds the current quota state for one requested model.
type APIKeyModelQuotaState struct {
	Model string
	Limit float64
	Used  float64
}

func NormalizeAPIKeyModelQuotaKey(model string) string {
	return strings.TrimSpace(model)
}

func NormalizeAPIKeyModelQuotaLimits(input map[string]float64) (map[string]float64, error) {
	if len(input) == 0 {
		return nil, nil
	}

	out := make(map[string]float64, len(input))
	for rawModel, rawLimit := range input {
		model := NormalizeAPIKeyModelQuotaKey(rawModel)
		if model == "" {
			return nil, fmt.Errorf("%w: model name is required", ErrInvalidModelQuotaLimit)
		}
		if rawLimit < 0 {
			return nil, fmt.Errorf("%w: model %q limit must be non-negative", ErrInvalidModelQuotaLimit, model)
		}
		if rawLimit == 0 {
			continue
		}
		out[model] = rawLimit
	}

	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func NormalizeAPIKeyModelQuotaUsed(input map[string]float64, limits map[string]float64) map[string]float64 {
	if len(input) == 0 || len(limits) == 0 {
		return nil
	}

	out := make(map[string]float64, len(input))
	for rawModel, rawUsed := range input {
		model := NormalizeAPIKeyModelQuotaKey(rawModel)
		if model == "" || rawUsed <= 0 {
			continue
		}
		if _, ok := limits[model]; !ok {
			continue
		}
		out[model] = rawUsed
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func (k *APIKey) HasModelQuotaLimits() bool {
	return k != nil && len(k.ModelQuotaLimits) > 0
}

func (k *APIKey) ModelQuotaLimitFor(model string) (float64, bool) {
	if k == nil {
		return 0, false
	}
	normalized := NormalizeAPIKeyModelQuotaKey(model)
	if normalized == "" || len(k.ModelQuotaLimits) == 0 {
		return 0, false
	}
	limit, ok := k.ModelQuotaLimits[normalized]
	if !ok || limit <= 0 {
		return 0, false
	}
	return limit, true
}

func (k *APIKey) ModelQuotaUsedFor(model string) float64 {
	if k == nil {
		return 0
	}
	normalized := NormalizeAPIKeyModelQuotaKey(model)
	if normalized == "" || len(k.ModelQuotaUsed) == 0 {
		return 0
	}
	used := k.ModelQuotaUsed[normalized]
	if used < 0 {
		return 0
	}
	return used
}

func (k *APIKey) GetModelQuotaRemaining(model string) (float64, bool) {
	limit, ok := k.ModelQuotaLimitFor(model)
	if !ok {
		return 0, false
	}
	remaining := limit - k.ModelQuotaUsedFor(model)
	if remaining < 0 {
		return 0, true
	}
	return remaining, true
}

func (k *APIKey) IsModelQuotaExhausted(model string) bool {
	limit, ok := k.ModelQuotaLimitFor(model)
	if !ok {
		return false
	}
	return k.ModelQuotaUsedFor(model) >= limit
}
