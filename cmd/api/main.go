package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/GTDGit/gtd_api/internal/cache"
	"github.com/GTDGit/gtd_api/internal/config"
	"github.com/GTDGit/gtd_api/internal/database"
	"github.com/GTDGit/gtd_api/internal/handler"
	"github.com/GTDGit/gtd_api/internal/middleware"
	"github.com/GTDGit/gtd_api/internal/models"
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/worker"
	"github.com/GTDGit/gtd_api/pkg/alterra"
	dfg "github.com/GTDGit/gtd_api/pkg/digiflazz"
	"github.com/GTDGit/gtd_api/pkg/kiosbank"
)

// main is the application entrypoint for the GTD API Gateway (Phase 1).
func main() {
	// 1. Load config
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// 2. Setup logger
	setupLogger(cfg.Env)
	log.Info().Str("env", cfg.Env).Msg("starting gtd api")

	// 3. Connect database
	db, err := database.Connect(&cfg.DB)
	if err != nil {
		log.Error().Err(err).Msg("database connection failed")
		fmt.Fprintf(os.Stderr, "database connection failed: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// 3a. Run migrations
	if err := runMigrations(db.DB); err != nil {
		log.Error().Err(err).Msg("migration failed")
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
	log.Info().Msg("migrations completed successfully")

	// 3b. Connect to Redis
	redisClient, err := cache.NewRedisClient(&cfg.Redis)
	if err != nil {
		log.Error().Err(err).Msg("redis connection failed")
		fmt.Fprintf(os.Stderr, "redis connection failed: %v\n", err)
		os.Exit(1)
	}
	defer redisClient.Close()
	log.Info().Msg("redis connected successfully")

	// 3c. Initialize inquiry cache
	inquiryCache := cache.NewInquiryCache(redisClient)

	// 4. Initialize Digiflazz clients (production & development)
	digiProd := dfg.NewClient(cfg.Digiflazz.Username, cfg.Digiflazz.KeyProduction)
	digiDev := dfg.NewClient(cfg.Digiflazz.Username, cfg.Digiflazz.KeyDevelopment)

	// 5. Initialize repositories
	clientRepo := repository.NewClientRepository(db)
	productRepo := repository.NewProductRepository(db)
	skuRepo := repository.NewSKURepository(db)
	trxRepo := repository.NewTransactionRepository(db)
	cbRepo := repository.NewCallbackRepository(db)
	adminRepo := repository.NewAdminUserRepository(db)
	territoryRepo := repository.NewTerritoryRepository(db)
	bankCodeRepo := repository.NewBankCodeRepository(db)
	ocrRepo := repository.NewOCRRepository(db)
	ppobProviderRepo := repository.NewPPOBProviderRepository(db)

	// 5a. Initialize PPOB provider clients
	var kioskbankClient *kiosbank.Client
	if cfg.Kiosbank.Username != "" {
		kioskbankClient = kiosbank.NewClient(kiosbank.Config{
			BaseURL:    cfg.Kiosbank.BaseURL,
			MerchantID: cfg.Kiosbank.MerchantID,
			CounterID:  cfg.Kiosbank.CounterID,
			AccountID:  cfg.Kiosbank.AccountID,
			Mitra:      cfg.Kiosbank.Mitra,
			Username:   cfg.Kiosbank.Username,
			Password:   cfg.Kiosbank.Password,
		})
	}

	var alterraClient *alterra.Client
	if cfg.Alterra.ClientID != "" && (cfg.Alterra.PrivateKeyPath != "" || cfg.Alterra.PrivateKeyPEM != "") {
		var err error
		alterraClient, err = alterra.NewClient(alterra.Config{
			BaseURL:        cfg.Alterra.BaseURL,
			ClientID:       cfg.Alterra.ClientID,
			PrivateKeyPath: cfg.Alterra.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Alterra.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Alterra client initialization failed - provider will be disabled")
		}
	}

	// 6. Initialize services
	authSvc := service.NewAuthService(clientRepo)
	adminAuthSvc := service.NewAdminAuthService(adminRepo)
	clientSvc := service.NewClientService(clientRepo)
	productSvc := service.NewProductService(productRepo, skuRepo)
	productMgmtSvc := service.NewProductManagementService(productRepo, skuRepo)
	callbackSvc := service.NewCallbackService(clientRepo, cbRepo, trxRepo)
	// NOTE: Old SyncService disabled - price sync now handled by ProviderSyncWorker
	// syncSvc := service.NewSyncService(digiProd, productRepo, skuRepo)
	trxSvc := service.NewTransactionService(trxRepo, productRepo, skuRepo, cbRepo, digiProd, digiDev, productSvc, callbackSvc, inquiryCache)

	// Wire up callback service to transaction service for immediate retry on webhook
	callbackSvc.SetTransactionRetrier(trxSvc)

	// Initialize Admin Transaction service
	adminTrxSvc := service.NewAdminTransactionService(trxRepo, cbRepo, productSvc, trxSvc, callbackSvc)

	// Initialize S3 service for Identity
	s3Svc, err := service.NewS3Service(&cfg.S3)
	if err != nil {
		log.Warn().Err(err).Msg("S3 service initialization failed - OCR file upload will be disabled")
	}

	// Initialize OCR service
	ocrSvc := service.NewOCRService(ocrRepo, territoryRepo, &cfg.Identity, s3Svc)

	// Initialize Liveness repository and service
	livenessRepo := repository.NewLivenessRepository(db)
	livenessSvc := service.NewLivenessService(livenessRepo, cfg)

	// Initialize FaceCompare repository and service
	faceCompareRepo := repository.NewFaceCompareRepository(db)
	faceCompareSvc := service.NewFaceCompareService(faceCompareRepo, s3Svc, cfg)

	// Initialize Provider Router for multi-provider PPOB
	providerRouter := service.NewProviderRouter(ppobProviderRepo)
	if kioskbankClient != nil {
		kiosbankAdapter := service.NewKiosbankProviderClient(kioskbankClient, kioskbankClient) // Same client for prod/dev
		providerRouter.RegisterProvider(models.ProviderKiosbank, kiosbankAdapter)
		log.Info().Msg("Kiosbank provider registered")
	}
	if alterraClient != nil {
		alterraAdapter := service.NewAlterraProviderClient(alterraClient, alterraClient) // Same client for prod/dev
		providerRouter.RegisterProvider(models.ProviderAlterra, alterraAdapter)
		log.Info().Msg("Alterra provider registered")
	}
	// Digiflazz is always available as backup
	digiAdapter := service.NewDigiflazzProviderClient(digiProd, digiDev)
	providerRouter.RegisterProvider(models.ProviderDigiflazz, digiAdapter)
	log.Info().Msg("Digiflazz provider registered (backup)")

	// Wire provider router to transaction service for multi-provider support
	trxSvc.SetProviderRouter(providerRouter)
	log.Info().Msg("Provider router connected to transaction service")

	// Update product service with provider-aware version for best price
	productSvc = service.NewProductServiceWithProviders(productRepo, skuRepo, ppobProviderRepo)

	// Initialize provider callback service
	providerCallbackSvc := service.NewProviderCallbackService(ppobProviderRepo, trxRepo, callbackSvc)

	// 7. Initialize handlers
	handlers := &Handlers{
		Health:            handler.NewHealthHandler(digiProd),
		Product:           handler.NewProductHandler(productSvc),
		Balance:           handler.NewBalanceHandler(digiProd),
		Transaction:       handler.NewTransactionHandler(trxSvc, productSvc),
		Webhook:           handler.NewWebhookHandler(callbackSvc, cfg.Digiflazz.WebhookSecret),
		Client:            handler.NewClientHandler(clientSvc),
		ProductManagement: handler.NewProductManagementHandler(productMgmtSvc),
		AdminTransaction:  handler.NewAdminTransactionHandler(adminTrxSvc),
		Auth:              handler.NewAuthHandler(adminAuthSvc),
		Territory:         handler.NewTerritoryHandler(territoryRepo),
		BankCode:          handler.NewBankCodeHandler(bankCodeRepo),
		OCR:               handler.NewOCRHandler(ocrSvc),
		Liveness:          handler.NewLivenessHandler(livenessSvc),
		FaceCompare:       handler.NewFaceCompareHandler(faceCompareSvc),
		PPOBProvider:      handler.NewPPOBProviderHandler(ppobProviderRepo),
		ProviderCallback:  handler.NewProviderCallbackHandler(providerCallbackSvc, cfg.Alterra.CallbackPublicKey),
	}

	// 8. Initialize middleware
	authMw := middleware.NewAuthMiddleware(authSvc)
	jwtMw := middleware.NewJWTMiddleware()

	// 9. Setup router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.CORSMiddleware())
	router.Use(middleware.LoggingMiddleware())
	setupRoutes(router, handlers, authMw, jwtMw)

	// 10. Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 11. Start workers
	// NOTE: Old SyncWorker disabled - price sync now handled by ProviderSyncWorker for all providers
	// go worker.NewSyncWorker(syncSvc, cfg.Worker.SyncInterval).Start(ctx)
	go worker.NewRetryWorker(trxRepo, callbackSvc, cfg.Worker.RetryInterval).Start(ctx)
	go worker.NewCallbackWorker(callbackSvc, cfg.Worker.CallbackInterval).Start(ctx)
	go worker.NewDigiflazzCallbackWorker(cbRepo, trxRepo, trxSvc, callbackSvc, cfg.Worker.DigiflazzCallbackInterval).Start(ctx)
	go worker.NewStatusCheckWorker(
		trxRepo, skuRepo, callbackSvc, digiProd, digiDev, providerRouter,
		cfg.Worker.StatusCheckInterval,
		cfg.Worker.StatusCheckStaleAfter,
		cfg.Worker.StatusCheckMaxAge,
	).Start(ctx)

	// Start provider price sync worker
	providerClients := providerRouter.GetClients()
	go worker.NewProviderSyncWorker(ppobProviderRepo, providerClients, cfg.Worker.SyncInterval).Start(ctx)

	// 12. Start HTTP server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Info().Str("port", cfg.Port).Msg("Starting server")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// 13. Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// 14. Cancel context to stop workers
	cancel()

	// 15. Shutdown HTTP server with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}
	log.Info().Msg("Server exited")
}

// Handlers groups all HTTP handlers used by the server.
type Handlers struct {
	Health            *handler.HealthHandler
	Product           *handler.ProductHandler
	Balance           *handler.BalanceHandler
	Transaction       *handler.TransactionHandler
	Webhook           *handler.WebhookHandler
	Client            *handler.ClientHandler
	ProductManagement *handler.ProductManagementHandler
	AdminTransaction  *handler.AdminTransactionHandler
	Auth              *handler.AuthHandler
	Territory         *handler.TerritoryHandler
	BankCode          *handler.BankCodeHandler
	OCR               *handler.OCRHandler
	Liveness          *handler.LivenessHandler
	FaceCompare       *handler.FaceCompareHandler
	PPOBProvider      *handler.PPOBProviderHandler
	ProviderCallback  *handler.ProviderCallbackHandler
}

// setupRoutes registers all routes.
func setupRoutes(router *gin.Engine, handlers *Handlers, authMiddleware *middleware.AuthMiddleware, jwtMiddleware *middleware.JWTMiddleware) {
	// Provider webhook endpoints
	router.POST("/webhook/digiflazz", handlers.Webhook.HandleDigiflazzCallback)
	router.POST("/webhook/kiosbank", handlers.ProviderCallback.HandleKiosbankCallback)
	router.POST("/webhook/alterra", handlers.ProviderCallback.HandleAlterraCallback)
	router.POST("/webhook/:provider", handlers.ProviderCallback.HandleGenericCallback)

	router.GET("/v1/health", handlers.Health.GetHealth)
	// API PPOB routes (protected with client API key)
	ppob := router.Group("/v1/ppob")
	ppob.Use(authMiddleware.Handle())
	{
		ppob.GET("/products", handlers.Product.GetProducts)
		ppob.GET("/balance", handlers.Balance.GetBalance)
		ppob.POST("/transaction", handlers.Transaction.CreateTransaction)
		ppob.GET("/transaction/:transactionId", handlers.Transaction.GetTransaction)
	}

	// Territory routes (protected with client API key)
	territory := router.Group("/v1/territory")
	territory.Use(authMiddleware.Handle())
	{
		territory.GET("/province", handlers.Territory.GetProvinces)
		territory.GET("/city", handlers.Territory.GetAllCities)
		territory.GET("/city/:province_code", handlers.Territory.GetCitiesByProvince)
		territory.GET("/district", handlers.Territory.GetAllDistricts)
		territory.GET("/district/:city_code", handlers.Territory.GetDistrictsByCity)
		territory.GET("/sub-district", handlers.Territory.GetAllSubDistricts)
		territory.GET("/sub-district/:district_code", handlers.Territory.GetSubDistrictsByDistrict)
		territory.GET("/postal-code", handlers.Territory.GetAllPostalCodes)
		territory.GET("/postal-code/district/:district_code", handlers.Territory.GetPostalCodesByDistrict)
		territory.GET("/postal-code/sub-district/:sub_district_code", handlers.Territory.GetPostalCodesBySubDistrict)
	}

	// Bank codes (protected with client API key)
	router.GET("/v1/bank-codes", authMiddleware.Handle(), handlers.BankCode.GetBankCodes)

	// Identity OCR routes (protected with client API key)
	identity := router.Group("/v1/identity")
	identity.Use(authMiddleware.Handle())
	{
		// OCR endpoints
		identity.POST("/ocr/ktp", handlers.OCR.KTPOCR)
		identity.POST("/ocr/npwp", handlers.OCR.NPWPOCR)
		identity.POST("/ocr/sim", handlers.OCR.SIMOCR)
		identity.GET("/ocr/:id", handlers.OCR.GetOCRByID)

		// Liveness endpoints
		identity.POST("/liveness/session", handlers.Liveness.CreateSession)
		identity.POST("/liveness/verify", handlers.Liveness.VerifyLiveness)
		identity.GET("/liveness/session/:sessionId", handlers.Liveness.GetSession)

		// Face Compare endpoints
		identity.POST("/compare", handlers.FaceCompare.CompareFaces)
		identity.GET("/compare/:id", handlers.FaceCompare.GetCompareByID)
	}

	// Admin routes
	admin := router.Group("/v1/admin")
	admin.POST("/auth/login", handlers.Auth.Login)
	admin.Use(jwtMiddleware.Handle())
	{
		// Client Management
		admin.POST("/clients", handlers.Client.CreateClient)
		admin.GET("/clients", handlers.Client.ListClients)
		admin.GET("/clients/:id", handlers.Client.GetClient)
		admin.GET("/clients/by-client-id/:client_id", handlers.Client.GetClientByClientID)
		admin.PUT("/clients/:id", handlers.Client.UpdateClient)
		admin.POST("/clients/:id/regenerate", handlers.Client.RegenerateKeys)

		// Product Management
		admin.GET("/products", handlers.ProductManagement.ListProducts)
		admin.GET("/products/categories", handlers.ProductManagement.GetCategories)
		admin.GET("/products/brands", handlers.ProductManagement.GetBrands)
		admin.POST("/products", handlers.ProductManagement.CreateProduct)
		admin.GET("/products/:id", handlers.ProductManagement.GetProduct)
		admin.PUT("/products/:id", handlers.ProductManagement.UpdateProduct)
		admin.DELETE("/products/:id", handlers.ProductManagement.DeleteProduct)

		// SKU Management
		admin.POST("/products/:id/skus", handlers.ProductManagement.CreateSKU)
		admin.GET("/products/:id/skus", handlers.ProductManagement.GetProductSKUs)
		admin.GET("/skus/:id", handlers.ProductManagement.GetSKU)
		admin.PUT("/skus/:id", handlers.ProductManagement.UpdateSKU)
		admin.DELETE("/skus/:id", handlers.ProductManagement.DeleteSKU)

		// Transaction Management (Admin)
		admin.GET("/transactions", handlers.AdminTransaction.ListTransactions)
		admin.GET("/transactions/stats", handlers.AdminTransaction.GetStats)
		admin.GET("/transactions/:id", handlers.AdminTransaction.GetTransaction)
		admin.GET("/transactions/:id/logs", handlers.AdminTransaction.GetTransactionLogs)
		admin.POST("/transactions/:id/retry", handlers.AdminTransaction.ManualRetry)

		// PPOB Provider Management
		admin.GET("/ppob/providers", handlers.PPOBProvider.ListProviders)
		admin.GET("/ppob/providers/:id", handlers.PPOBProvider.GetProvider)
		admin.PUT("/ppob/providers/:id/status", handlers.PPOBProvider.UpdateProviderStatus)

		// PPOB Provider SKU Management
		admin.GET("/ppob/providers/:id/skus", handlers.PPOBProvider.ListProviderSKUs)
		admin.POST("/ppob/providers/:id/skus", handlers.PPOBProvider.CreateProviderSKU)
		admin.GET("/ppob/products/:productId/provider-skus", handlers.PPOBProvider.GetProviderSKUsByProduct)
		admin.PUT("/ppob/skus/:id", handlers.PPOBProvider.UpdateProviderSKU)
		admin.DELETE("/ppob/skus/:id", handlers.PPOBProvider.DeleteProviderSKU)

		// PPOB Provider Health
		admin.GET("/ppob/health", handlers.PPOBProvider.GetAllProviderHealthToday)
		admin.GET("/ppob/providers/:id/health", handlers.PPOBProvider.GetProviderHealth)
	}
}

// runMigrations runs database migrations using golang-migrate.
func runMigrations(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("could not create migration driver: %w", err)
	}

	m, err := migrate.NewWithDatabaseInstance(
		"file://migrations",
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("could not create migration instance: %w", err)
	}

	// Run migrations
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("could not run migrations: %w", err)
	}

	return nil
}

func setupLogger(env string) {
	if env == "production" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}
