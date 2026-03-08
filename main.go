package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	appcontainer "github.com/hotkhwan/gateway-api/internal/app"
	"github.com/hotkhwan/gateway-api/internal/configruntime"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/optionsrepo"
	_ "github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/klivesvc"
	"github.com/hotkhwan/gateway-api/models/systemmod"

	"github.com/hotkhwan/gateway-api/internal/kafka/deliverycons"
	"github.com/hotkhwan/gateway-api/internal/kafka/kctrlcons"
	"github.com/hotkhwan/gateway-api/internal/kafka/klivecorns"
	"github.com/hotkhwan/gateway-api/internal/kafka/kschcorns"
	"github.com/hotkhwan/gateway-api/internal/kafka/kwatchcons"
	"github.com/hotkhwan/gateway-api/internal/kafka/normalizedcons"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/middleware"
	"github.com/hotkhwan/gateway-api/utils"
	"github.com/hotkhwan/gateway-api/utils/authutil"

	"github.com/hotkhwan/gateway-api/internal/kafka/atacons"
	"github.com/hotkhwan/gateway-api/internal/kafka/authzcons"
	"github.com/hotkhwan/gateway-api/internal/kafka/iwowncons"
	"github.com/hotkhwan/gateway-api/internal/kafka/kaicons"

	"github.com/hotkhwan/gateway-api/config"
	_ "github.com/hotkhwan/gateway-api/docs"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kcontrolmsg"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kwatchmsg"
	"github.com/hotkhwan/gateway-api/internal/services/crimes"
	"github.com/hotkhwan/gateway-api/router"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/contrib/otelfiber/v2"
	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	fiberSwagger "github.com/swaggo/fiber-swagger" // ✅ ใช้ของ swaggo
)

// @title           Klynx API v2
// @version         1.0
// @description     REST API for Klynx system
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @Security BearerAuth
// @name Authorization
// @Tags 1.Auth
// @Tags 2.devices
// @Tags 3.kcontrol
func main() {
	env := flag.String("env", "dev", "Environment: dev / uat / prod")
	flag.Parse()
	envFile := fmt.Sprintf(".env.%s", *env)

	// ✅ โหลด env ก่อน logger.Init()
	if err := godotenv.Load(envFile); err != nil {
		fmt.Printf("❌ Failed to load env file: %s\n", err)
		os.Exit(1)
	}

	basePath := os.Getenv("BASE_PATH")
	if basePath == "" {
		basePath = "/api/v1" // fallback default
	}
	iwownPath := os.Getenv("IWOWN_PATH")
	if iwownPath == "" {
		iwownPath = "/4g" // fallback default
	}

	ctx := context.Background()
	// ✅ Init configs
	logger.Init()
	config.InitTelemetry(ctx)
	config.InitKafka()
	config.InitMongo()
	config.InitPermifyRest()
	config.InitPermifygRPC()
	config.InitRedis()
	config.InitS3()
	// config.InitLLMConfig()
	// config.InitTelemetry()
	// kcontrolmsg.InitMQTT()
	// kwatchmsg.InitMQTT()
	log := logger.Boot("bootstrap", "main")
	log.Info().Str("sub", envFile).Msg("📂 Loading env file")

	// ใน main.go หลังจาก config.InitMongo()
	cfgRepo := optionsrepo.New(config.DB)
	effective, err := cfgRepo.LoadEffective(context.Background())
	if err != nil {
		// fallback แบบแข็ง (ปลอดภัย)
		effective = &systemmod.EffectiveConfig{
			BasePath:             "/api/v1",
			IwownPath:            "/4g",
			AuditCaptureResponse: "errors",
			AuditMaxRespBytes:    16384,
			AuditCaptureJSONOnly: true,
			AuditRetentionDays:   90,
			MaxRecordRequest:     10000,
			KafkaPublishTimeout:  "5s",
			KwatchBatchSize:      5000,
		}
	}

	// ใช้ค่า effective.AuditRetentionDays ตั้ง TTL index
	if err := authzrepo.EnsureAuditIndexes(context.Background(), config.DB, effective.AuditRetentionDays); err != nil {
		log.Printf("EnsureAuditIndexes error: %v\n", err)
	}

	optsRepo := optionsrepo.New(config.DB)
	// dynamic cache TTL 30s
	effCache := configruntime.NewCachedEffectiveLoader(optsRepo, 30*time.Second)
	middleware.SetEffectiveGetter(effCache.Get)
	// สร้าง TTL index ครั้งแรกด้วยค่าจาก effective ปัจจุบัน
	if eff, err := optsRepo.LoadEffective(context.Background()); err == nil {
		_ = authzrepo.EnsureAuditIndexes(context.Background(), config.DB, eff.AuditRetentionDays)
	}
	// Start Consumer Real-time
	schemaVersion, err := authzsvc.ApplySchema(ctx)
	if err != nil {
		log.Warn().Msg("⚠️ Apply schema failed, skipping initial sync")
	} else {
		if err := authzsvc.InitialSyncRelationships(ctx, schemaVersion); err != nil {
			log.Warn().Msg("⚠️ Initial sync failed, using real-time events only")
		}
	}

	// Redis podID Generation
	podID := os.Getenv("HOSTNAME")
	if podID == "" {
		podID = fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	// Start Redis Subscribe
	klivesvc.StartStreamExpiryWatcher(context.Background(), podID)

	// Start Kafka Consumers
	go atacons.StartKafkaATAConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_TOPIC_ATA", "ata.events"))
	go authzcons.StartKafkaAuthzRelationshipConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_AUTHZ_TOPIC", "authz.relationship.updated"))
	go kctrlcons.StartKafkaAlarmConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_ALARM", "kcontrol.alarms"))
	go atacons.StartKafkaATAConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_TOPIC_ATA", "ata.events-feature"))
	go kctrlcons.StartKafkaEventConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_EVENT", "kcontrol.events"))
	go kctrlcons.StartKafkaHealthConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_HEALTH", "kcontrol.health"))
	go kctrlcons.StartKafkaSensorConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_SENSOR", "kcontrol.sensor"))
	go kaicons.StartKafkaDetectConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_DETECTION_TOPIC", "tp.detection"))
	go klivecorns.StartKliveConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_LIVE_TOPIC", "klive.palyer"))
	go kschcorns.StartKsearchConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KSEARCH_TOPIC", "ksearch.video"))
	go kwatchcons.StartWatchlistConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KWATCH_TOPIC", "kwatch.watchlist"))
	go kwatchcons.StartWatchlistConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KWATCH_SYNC_TOPIC", "kwatch.watchlist.sync"))
	go iwowncons.StartKafkaIwownConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_TOPIC_IWOWN", "kwatch4g.iwown"))
	// Delivery consumer started after container is built — deps come from container
	// (wired below after container is created)

	go normalizedcons.StartNormalizerConsumer(ctx, normalizedcons.ConsumerDeps{
		EventDetailsRepo: ingestdetailsrepo.NewEventDetailsRepo(),
		TemplateRepo:     ingestrepo.NewMappingTemplateRepo(),
		GeoCfg:           normalizedcons.DefaultGeoConfig(),
		S3BucketKey:      os.Getenv("S3_EVENTS_BUCKET_KEY"),
		Logger:           logger.WithMeta("normalizer", "consumer"),
	})

	app := fiber.New(fiber.Config{
		ReadBufferSize: 16 * 1024,
		BodyLimit:      50 * 1024 * 1024,
		StrictRouting:  false,
		Prefork:        false, // ✅ เปิดใช้งาน Prefork multi-core เพื่อรองรับ TPS สูง HTTP API (Stateless) on k8s ใช้ 1 core ต่อ Pod → scale ด้วย HPA ตาม CPU / QPS
		// ✅ ให้ Fiber ใช้ X-Forwarded-For เป็นแหล่ง IP และเปิดตรวจ proxy ที่เชื่อถือได้
		ProxyHeader:             fiber.HeaderXForwardedFor,
		EnableTrustedProxyCheck: true,
		TrustedProxies: []string{
			"10.42.0.0/16",   // Pod CIDR (ปรับตามคลัสเตอร์)
			"192.168.0.0/16", // LAN/Node
			"127.0.0.1/32",
		},
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			// 1️⃣ Fiber error เช่น 404, 405
			if e, ok := err.(*fiber.Error); ok {
				return c.Status(e.Code).JSON(fiber.Map{
					"code":    "BAD_REQUEST",
					"message": e.Message,
					"status":  false,
				})
			}

			// 2️⃣ Validation error
			if ve, ok := err.(validator.ValidationErrors); ok {
				errors := make([]string, len(ve))
				for i, fe := range ve {
					errors[i] = fe.Field() + " failed on " + fe.Tag()
				}
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"code":    "VALIDATION_ERROR",
					"message": errors,
					"status":  false,
				})
			}

			// 3️⃣ Unexpected error → 500
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"code":    "INTERNAL_ERROR",
				"message": err.Error(),
				"status":  false,
			})
		},
	})
	app.Use(recovermw.New())
	app.Use(otelfiber.Middleware())
	app.Use(middleware.TraceHeader())
	app.Use(logger.FiberLogger())

	api := app.Group(basePath)
	// ✅ สร้าง container ครั้งเดียว
	container := appcontainer.NewContainer()

	// Start delivery consumer (template-driven dispatch to webhook/LINE/Discord/Telegram)
	go deliverycons.StartDeliveryConsumer(ctx, container.DeliveryDeps)

	// ✅ router ที่ migrate แล้ว — รับ container
	router.RegisterAuthzNewRoutes(api, container)
	router.RegisterResourceRoutes(api, container)
	// ✅ router เก่าที่ยังไม่ migrate — ยังทำงานได้ปกติ
	router.RegisterAPIATA(api)
	router.RegisterAuthRoutes(api)
	router.RegisterAuthzDebugRoutes(api)
	router.RegisterBIRoutes(api)
	router.RegisterKcontrolDashboard(api)
	router.RegisterDeviceSyncRoutes(api)
	router.RegisterFacesCCTVRoutes(api)
	router.RegisterGroupRoutes(api)
	router.RegisterImageProxy(api)
	router.RegisterKcontrolRoutes(api)
	router.RegisterKsearchRoutes(api)
	router.RegisterKwatchRoutes(api)
	router.RegisterMapsRoutes(api)
	router.RegisterMedia(api)
	router.RegisterOptRoutes(api)
	// router.RegisterResourceRoutes(api)
	router.RegisterSystemRoutes(api)
	router.RegisterThirdAPIRoutes(api)
	router.RegisterUserRoutes(api)
	router.RegisterMemberRoutes(api)

	router.RegisterHookIboc(api)
	router.RegisterHookzkt(api)
	router.RegisterHookATA(api)
	router.RegisterSubscriptionRoutes(api, container)

	// ---------- Ingest hot-path: POST /events/:orgId (no JWT, root level) ----------
	router.RegisterIngestEventsRoutes(app, container)
	// ---------- Ingest config: GET|POST /api/v1/ingest (JWT + X-Active-Org) ----------
	router.RegisterIngestRoutes(api, container)

	// ---------- Delivery Targets domain ----------
	router.RegisterTargetsRoutes(api, container)

	iwownapi := app.Group(iwownPath)
	router.RegisterHookIwownAPI(iwownapi)

	// go func() {
	// 	interval := utils.GetEnvDurationSec("KCTRL_WATCHDOG_INTERVAL", 5) // default 5s
	// 	log := logger.WithMeta("bootstrap", "watchdog")

	// 	log.Info().
	// 		Dur("interval", interval).
	// 		Msg("🕒 Starting kcontrol watchdog loop")

	// 	ticker := time.NewTicker(interval)
	// 	defer ticker.Stop()

	// 	for range ticker.C {
	// 		kcontrolmsg.CheckDeviceStatus()
	// 	}
	// }()

	// ✅ Swagger
	swaggerPath := os.Getenv("SWAGGER_PATH")
	if swaggerPath == "" {
		swaggerPath = "/docs"
	}
	api.Get(swaggerPath+"/*", fiberSwagger.WrapHandler)
	// api.Get("/docs/*", fiberSwagger.WrapHandler)

	app.Use(func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"message": "Route not found",
			"path":    c.OriginalURL(),
		})
	})
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Info().Msg("🛑 Shutting down Kafka writers...")
		config.LoadATAConfigFromEnv()
		config.CloseKafka()
		config.DisconnectMongo()
		kcontrolmsg.DisconnectMQTT()
		kwatchmsg.DisconnectMQTT()
		config.DisconnectTelemetry()
		config.DisconnectRedis()
		os.Exit(0)
	}()

	jwksURL := os.Getenv("KEYCLOAK_JWKS_URL")
	if err := authutil.InitJWKS(jwksURL); err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to initialize JWKS")
	}

	// if _, err := os.Stat("/app/crimes"); err != nil {
	// 	log.Error().Err(err).Msg("cannot stat /app/crimes")
	// } else {
	// 	log.Info().Msg("✅ /app/crimes accessible")

	// }

	// ✅ Watch crimes dir
	coll := config.DB.Collection("kwatch_watchlist")
	// go crimes.WatchCrimesDir(ctx, "/app/crimes", coll)
	idxLog := logger.Boot("bootstrap", "indexes")
	if err := crimes.EnsureCrimesIndexes(context.Background(), coll); err != nil {
		idxLog.Error().Err(err).Msg("❌ EnsureCrimesIndexes failed")
	} else {
		idxLog.Info().Msg("✅ Crimes indexes ensured")
	}
	// watchLog := logger.Boot("crimes", "watcher")
	// go func() {
	// 	if err := crimes.WatchCrimesDir(ctx, "/app/crimes", coll); err != nil {
	// 		watchLog.Error().Err(err).Msg("watcher exited")
	// 	}
	// }()

	log.Info().Msg("✅ Service is already started")
	// Start serverCurrentSchemaVersion
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000" // fallback default
	}

	if err := app.Listen(":" + port); err != nil {
		log.Fatal().Err(err).Msg("❌ Failed to start Fiber server")
	}

	retention := authzrepo.LoadAuditRetentionDays() // default 90
	if err := authzrepo.EnsureAuditIndexes(context.Background(), config.DB, retention); err != nil {
		log.Printf("EnsureAuditIndexes error: %v\n", err)
	}
}
