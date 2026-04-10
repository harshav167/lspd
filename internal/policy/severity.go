package policy

import "github.com/harsha/lspd/internal/config"

func allowSource(cfg config.PolicyConfig, source string) bool {
	if len(cfg.AllowedSources) > 0 {
		allowed := false
		for _, item := range cfg.AllowedSources {
			if item == source {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	for _, item := range cfg.DeniedSources {
		if item == source {
			return false
		}
	}
	return true
}
