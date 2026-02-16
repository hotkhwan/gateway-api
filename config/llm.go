// config/llm.go
package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/kschmod"
)

var (
	llmCfg  *kschmod.LLMConfig
	llmOnce sync.Once
)

// InitLLMConfig โหลดค่า config จาก env
func InitLLMConfig() *kschmod.LLMConfig {
	llmOnce.Do(func() {
		log := logger.Boot("llm", "config-InitLLM")

		url := os.Getenv("SEARCH_LLM_URL")
		if url == "" {
			log.Warn().Msg("⚠️ SEARCH_LLM_URL not set, local LLM disabled")
		}

		key := os.Getenv("OPENAI_API_KEY")
		enable := key != "" && os.Getenv("ENABLE_GPT_SUMMARY") != "false"

		llmCfg = &kschmod.LLMConfig{
			MyLLMUrl:        url,
			EnableGPT:       enable,
			OpenAIKey:       key,
			OpenAIModel:     getEnv("OPENAI_MODEL", "gpt-4o-mini"),
			MaxSummaryToken: getEnvInt("OPENAI_MAX_TOKENS", 200),
		}

		log.Info().
			Str("llmUrl", llmCfg.MyLLMUrl).
			Bool("enableGPT", llmCfg.EnableGPT).
			Str("model", llmCfg.OpenAIModel).
			Int("maxTokens", llmCfg.MaxSummaryToken).
			Msg("✅ LLM config initialized")
	})
	return llmCfg
}

func GetLLMConfig() *kschmod.LLMConfig {
	if llmCfg == nil {
		return InitLLMConfig()
	}
	return llmCfg
}

// ---- helpers ----
func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return def
}
