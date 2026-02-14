// config/mongo.go
package config

import (
	"context"
	"klynx/internal/logger"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient *mongo.Client
	DB          *mongo.Database
)

func InitMongo() {
	log := logger.Boot("mongoDB", "config-InitMongo")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	uri := os.Getenv("MONGO_URI")
	dbName := os.Getenv("MONGO_DB") // ✅ เพิ่มการอ่านชื่อ DB
	if dbName == "" {
		dbName = "klynx" // fallback
	}

	log.Debug().
		Str("uri", uri).
		Msg("Connecting to MongoDB")

	clientOpts := options.Client().
		ApplyURI(uri).
		SetMaxPoolSize(30).
		SetMinPoolSize(10).
		SetServerSelectionTimeout(15 * time.Second).
		SetCompressors([]string{"zstd", "snappy"}). // ลดปริมาณข้อมูลข้ามเน็ต
		SetHeartbeatInterval(10 * time.Second)
	client, err := mongo.Connect(ctx, clientOpts)

	// client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatal().Err(err).Msg("Mongo connect error")
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatal().Err(err).Msg("Mongo ping error")
	}

	MongoClient = client
	DB = client.Database(dbName) // ✅ กำหนด DB

	log.Info().Msg("✅ MongoDB connected to " + dbName)
}

func DisconnectMongo() {
	log := logger.Boot("mongoDB", "config-DisconnectMongo")
	if MongoClient != nil {
		if err := MongoClient.Disconnect(context.Background()); err != nil {
			log.Warn().Msg("❌ Mongo disconnect failed")
		} else {
			log.Info().Msg("✅ Mongo disconnected")
		}
	}
}
