// config/redis.go
package config

import (
	"os"
	"strconv"

	"github.com/hotkhwan/gateway-api/internal/logger"

	"github.com/redis/go-redis/v9"
)

var Redis *redis.Client

func InitRedis() {
	log := logger.Boot("redis", "config-InitRedis")
	addr := os.Getenv("REDIS_URI")
	if addr == "" {
		log.Warn().
			Msg("❌ REDIS_URI not found, using default localhost:6379")
		addr = "localhost:6379"
	}

	password := os.Getenv("REDIS_PASSWORD")
	dbStr := os.Getenv("REDIS_DB")
	db, err := strconv.Atoi(dbStr)
	if err != nil {
		log.Warn().
			Str("value", dbStr).
			Msg("Invalid REDIS_DB, fallback to 0")
		db = 0
	}

	Redis = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	log.Info().
		Msg("✅ Redis client initialized")
}

func DisconnectRedis() {
	log := logger.Boot("redis", "config-DisconnectRedis")
	if Redis != nil {
		if err := Redis.Close(); err != nil {
			log.Warn().
				Err(err).
				Msg("⚠️ Error closing Redis")
		} else {
			log.Info().
				Msg("🛑 Redis connection closed")
		}
	}
}
