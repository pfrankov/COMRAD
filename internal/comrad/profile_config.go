package comrad

import "fmt"

type ProfileConfig struct {
	ID              string                        `json:"profileId" yaml:"profileId"`
	Model           string                        `json:"model" yaml:"model"`
	Kind            string                        `json:"kind" yaml:"kind"`
	ComputeCost     int64                         `json:"computeCost" yaml:"computeCost"`
	Runtime         ProfileRuntimeConfig          `json:"runtime" yaml:"runtime"`
	Requirements    ProfileRequirementsConfig     `json:"requirements" yaml:"requirements"`
	Warmable        bool                          `json:"warmable" yaml:"warmable"`
	RuntimeVariants []ProfileRuntimeVariantConfig `json:"runtimeVariants,omitempty" yaml:"runtimeVariants,omitempty"`
}

type ProfileRuntimeConfig struct {
	Adapter        string             `json:"adapter" yaml:"adapter"`
	ModelArtifacts []string           `json:"modelArtifacts" yaml:"modelArtifacts"`
	ContextTokens  int                `json:"contextTokens" yaml:"contextTokens"`
	LlamaCpp       LlamaCppParameters `json:"llamaCpp,omitempty" yaml:"llamaCpp,omitempty"`
}

type ProfileRuntimeVariantConfig struct {
	ID             string                    `json:"variantId" yaml:"variantId"`
	Target         string                    `json:"target" yaml:"target"`
	Adapter        string                    `json:"adapter" yaml:"adapter"`
	ModelArtifacts []string                  `json:"modelArtifacts" yaml:"modelArtifacts"`
	ContextTokens  int                       `json:"contextTokens" yaml:"contextTokens"`
	LlamaCpp       LlamaCppParameters        `json:"llamaCpp,omitempty" yaml:"llamaCpp,omitempty"`
	Requirements   *ProfileRequirementsConfig `json:"requirements,omitempty" yaml:"requirements,omitempty"`
}

type ProfileRequirementsConfig struct {
	Target             string   `json:"target" yaml:"target"`
	RAMBytes           int64    `json:"ramBytes,omitempty" yaml:"ramBytes,omitempty"`
	VRAMBytes          int64    `json:"vramBytes,omitempty" yaml:"vramBytes,omitempty"`
	UnifiedMemoryBytes int64    `json:"unifiedMemoryBytes,omitempty" yaml:"unifiedMemoryBytes,omitempty"`
	DiskBytes          int64    `json:"diskBytes,omitempty" yaml:"diskBytes,omitempty"`
	RequireTags        []string `json:"requireTags,omitempty" yaml:"requireTags,omitempty"`
	MinimumWorker      string   `json:"minimumWorkerVersion,omitempty" yaml:"minimumWorkerVersion,omitempty"`
}

func profileFromConfig(cfg ProfileConfig) (WorkloadProfile, error) {
	if err := validateProfileConfig(cfg); err != nil {
		return WorkloadProfile{}, err
	}
	artifacts := make([]string, 0, len(cfg.Runtime.ModelArtifacts))
	for _, id := range cfg.Runtime.ModelArtifacts {
		artifacts = append(artifacts, NormalizeSHA256(id))
	}
	variants := make([]RuntimeModelVariant, 0, len(cfg.RuntimeVariants))
	for _, v := range cfg.RuntimeVariants {
		variantArtifacts := make([]string, 0, len(v.ModelArtifacts))
		for _, id := range v.ModelArtifacts {
			variantArtifacts = append(variantArtifacts, NormalizeSHA256(id))
		}
		var req *Requirements
		if v.Requirements != nil {
			req = &Requirements{
				Target:             v.Requirements.Target,
				RAMBytes:           v.Requirements.RAMBytes,
				VRAMBytes:          v.Requirements.VRAMBytes,
				UnifiedMemoryBytes: v.Requirements.UnifiedMemoryBytes,
				DiskBytes:          v.Requirements.DiskBytes,
				RequireTags:        v.Requirements.RequireTags,
				MinimumWorker:      v.Requirements.MinimumWorker,
			}
		}
		var variantLLM *LLMProfile
		if v.ContextTokens > 0 {
			variantLLM = &LLMProfile{ContextTokens: v.ContextTokens}
		}
		variants = append(variants, RuntimeModelVariant{
			ID:             v.ID,
			Name:           "",
			Target:         v.Target,
			RuntimeAdapter: v.Adapter,
			Artifacts:      variantArtifacts,
			Requirements:   req,
			LLM:            variantLLM,
			Runtime:        RuntimeParameters{LlamaCpp: v.LlamaCpp},
		})
	}
	return WorkloadProfile{
		ID:             cfg.ID,
		Name:           cfg.Model,
		Alias:          cfg.Model,
		LogicalModel:   cfg.Model,
		Kind:           cfg.Kind,
		RuntimeAdapter: cfg.Runtime.Adapter,
		Artifacts:      artifacts,
		Requirements: &Requirements{
			Target:             cfg.Requirements.Target,
			RAMBytes:           cfg.Requirements.RAMBytes,
			VRAMBytes:          cfg.Requirements.VRAMBytes,
			UnifiedMemoryBytes: cfg.Requirements.UnifiedMemoryBytes,
			DiskBytes:          cfg.Requirements.DiskBytes,
			RequireTags:        cfg.Requirements.RequireTags,
			MinimumWorker:      cfg.Requirements.MinimumWorker,
		},
		LLM:                   &LLMProfile{ContextTokens: cfg.Runtime.ContextTokens},
		Runtime:               RuntimeParameters{LlamaCpp: cfg.Runtime.LlamaCpp},
		RuntimeVariants:       variants,
		ComputeCost:           cfg.ComputeCost,
		Warmable:              cfg.Warmable,
		MaxConcurrencyPerSlot: 1,
	}, nil
}

func validateProfileConfig(cfg ProfileConfig) error {
	if cfg.ID == "" {
		return fmt.Errorf("profileId is required")
	}
	if cfg.Model == "" {
		return fmt.Errorf("model is required")
	}
	if cfg.Kind != "llm.chat" {
		return fmt.Errorf("kind must be llm.chat")
	}
	hasVariants := len(cfg.RuntimeVariants) > 0
	if cfg.Runtime.Adapter == "" && !hasVariants {
		return fmt.Errorf("runtime.adapter is required")
	}
	if len(cfg.Runtime.ModelArtifacts) == 0 && !hasVariants {
		return fmt.Errorf("runtime.modelArtifacts is required when no runtimeVariants are defined")
	}
	if cfg.Runtime.ContextTokens <= 0 && !hasVariants {
		return fmt.Errorf("runtime.contextTokens is required")
	}
	if cfg.ComputeCost < 0 {
		return fmt.Errorf("computeCost must be non-negative")
	}
	if err := validateLlamaServerProfileArgs(cfg.Runtime.LlamaCpp.Args); err != nil {
		return err
	}
	if cfg.Requirements.Target == "" {
		return fmt.Errorf("requirements.target is required")
	}
	return validateProfileVariants(cfg.RuntimeVariants)
}

func validateProfileVariants(variants []ProfileRuntimeVariantConfig) error {
	for i, v := range variants {
		if v.ID == "" {
			return fmt.Errorf("runtimeVariants[%d].variantId is required", i)
		}
		if v.Target == "" {
			return fmt.Errorf("runtimeVariants[%d].target is required", i)
		}
		if v.Adapter == "" {
			return fmt.Errorf("runtimeVariants[%d].adapter is required", i)
		}
		if err := validateLlamaServerProfileArgs(v.LlamaCpp.Args); err != nil {
			return fmt.Errorf("runtimeVariants[%d].llamaCpp.args: %w", i, err)
		}
	}
	return nil
}

func validateWorkloadProfileRuntime(profile WorkloadProfile) error {
	if isLlamaCppAdapter(profile.RuntimeAdapter) {
		if err := validateLlamaServerProfileArgs(profile.Runtime.LlamaCpp.Args); err != nil {
			return err
		}
	}
	for _, variant := range profile.RuntimeVariants {
		normalized := normalizeVariant(profile, variant)
		if !isLlamaCppAdapter(normalized.RuntimeAdapter) {
			continue
		}
		if err := validateLlamaServerProfileArgs(normalized.Runtime.LlamaCpp.Args); err != nil {
			return err
		}
	}
	return nil
}
