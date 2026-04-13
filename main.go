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
	"github.com/hotkhwan/gateway-api/internal/grpc/workspacegrpc"
	"github.com/hotkhwan/gateway-api/internal/configruntime"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/optionsrepo"
	_ "github.com/hotkhwan/gateway-api/internal/repo/subscriprepo"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/internal/services/klivesvc"
	"github.com/hotkhwan/gateway-api/models/systemmod"

	"github.com/hotkhwan/gateway-api/internal/kafka/deliverycons"
	"github.com/hotkhwan/gateway-api/internal/kafka/entitlementcons"
	"github.com/hotkhwan/gateway-api/internal/kafka/klynxdeliverycons"
	"github.com/hotkhwan/gateway-api/internal/kafka/orglifecyclecons"
	// "github.com/hotkhwan/gateway-api/internal/kafka/kctrlcons"
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
	gatewaydocs "github.com/hotkhwan/gateway-api/docs"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kcontrolmsg"
	"github.com/hotkhwan/gateway-api/internal/mqtt/kwatchmsg"
	"github.com/hotkhwan/gateway-api/internal/services/crimes"
	"github.com/hotkhwan/gateway-api/router"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	recovermw "github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/joho/godotenv"

	"github.com/hotkhwan/gateway-api/internal/otelfiber"
	internalswagger "github.com/hotkhwan/gateway-api/internal/swagger"
)

// @title           phibek API
// @version         1.0
// @description     Event ingestion, normalization, and delivery gateway
// @BasePath        /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @Security BearerAuth
// @name Authorization
// @Tags 1.Auth
// @Tags 2.Workspaces
// @Tags 3.Ingest
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
	// Propagate runtime values into the generated swagger spec.
	gatewaydocs.SwaggerInfo.BasePath = basePath
	gatewaydocs.SwaggerInfo.Version = Version

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
	authzsvc.BootstrapPlatformAdmins(ctx)
	authzsvc.BackfillWorkspaceTuples(ctx)

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
	// go kctrlcons.StartKafkaAlarmConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_ALARM", "kcontrol.alarms"))
	go atacons.StartKafkaATAConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_TOPIC_ATA", "ata.events-feature"))
	// go kctrlcons.StartKafkaEventConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_EVENT", "kcontrol.events"))
	// go kctrlcons.StartKafkaHealthConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_HEALTH", "kcontrol.health"))
	// go kctrlcons.StartKafkaSensorConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("IOT_TOPIC_SENSOR", "kcontrol.sensor"))
	go kaicons.StartKafkaDetectConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_DETECTION_TOPIC", "tp.detection"))
	go klivecorns.StartKliveConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_LIVE_TOPIC", "klive.palyer"))
	go kschcorns.StartKsearchConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KSEARCH_TOPIC", "ksearch.video"))
	go kwatchcons.StartWatchlistConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KWATCH_TOPIC", "kwatch.watchlist"))
	go kwatchcons.StartWatchlistConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_KWATCH_SYNC_TOPIC", "kwatch.watchlist.sync"))
	go iwowncons.StartKafkaIwownConsumer(os.Getenv("KAFKA_BROKER"), utils.Getenv("KAFKA_TOPIC_IWOWN", "kwatch4g.iwown"))
	// Delivery consumer started after container is built — deps come from container
	// (wired below after container is created)

	// normalizer consumer started after container — uses container.NormalizerDeps
	// (includes entitlement gate, authz gate, EventBridge routing)
	// started below after appcontainer.NewContainer()

	app := fiber.New(fiber.Config{
		ReadBufferSize: 16 * 1024,
		BodyLimit:      50 * 1024 * 1024,
		StrictRouting:  false,
		// Prefork removed in Fiber v3 — use HPA / multiple pods instead
		// ✅ ให้ Fiber ใช้ X-Forwarded-For เป็นแหล่ง IP และเปิดตรวจ proxy ที่เชื่อถือได้
		ProxyHeader: fiber.HeaderXForwardedFor,
		TrustProxy:  true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Proxies: []string{
				"10.42.0.0/16",   // Pod CIDR (ปรับตามคลัสเตอร์)
				"192.168.0.0/16", // LAN/Node
				"127.0.0.1/32",
			},
		},
		ErrorHandler: func(c fiber.Ctx, err error) error {
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

	// ✅ Base path info endpoint
	api.All("/", middleware.AllowMethods("GET"))
	api.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"code":    "SUCCESS",
			"message": "Gateway API",
			"status":  true,
			"details": fiber.Map{
				"service": "gateway",
				"version": Version,
			},
		})
	})

	// ✅ สร้าง container ครั้งเดียว
	container := appcontainer.NewContainer()

	// Start normalizer consumer (uses container deps — includes gates + EventBridge routing)
	go normalizedcons.StartNormalizerConsumer(ctx, container.NormalizerDeps)

	// Start delivery consumer (template-driven dispatch to webhook/LINE/Discord/Telegram)
	go deliverycons.StartDeliveryConsumer(ctx, container.DeliveryDeps)

	// Start entitlement consumer — syncs klynx.entitlement.snapshot.v1 into Redis TTL cache
	go entitlementcons.StartEntitlementConsumer(container.EntitlementService)

	// Start org lifecycle consumers — provision/suspend EVENTS workspace on klynx org events
	orglifecyclecons.StartOrgLifecycleConsumers(container.WorkspaceService)

	// Start gRPC workspace provisioning server (appliance mode: klynx calls EVENTS-api directly)
	go func() {
		if err := workspacegrpc.Start(ctx, container.WorkspaceService, container.TargetService); err != nil {
			log.Error().Err(err).Msg("❌ gRPC workspace server exited")
		}
	}()

	// Start klynx delivery consumer — saasPublic only (POSTs events.delivery.v1 → klynx webhook)
	if os.Getenv("DEPLOYMENT_PROFILE") == "saasPublic" {
		go klynxdeliverycons.StartKlynxDeliveryConsumer(ctx)
	}

	// ✅ router ที่ migrate แล้ว — รับ container
	router.RegisterWorkspaceRoutes(api, container)
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

	// ---------- Workspace-scoped resources (targets, bindings, ingest/msg templates) ----------
	router.RegisterWorkspaceResourceRoutes(api, container)

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
	api.Get(swaggerPath+"/*", internalswagger.New(internalswagger.Config{
		Title: "Gateway API",
	}))

	app.Use(func(c fiber.Ctx) error {
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

	log.Info().Str("version", Version).Str("basePath", basePath).Msg("✅ Service is already started")
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
