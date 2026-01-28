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
	"github.com/GTDGit/gtd_api/internal/repository"
	"github.com/GTDGit/gtd_api/internal/service"
	"github.com/GTDGit/gtd_api/internal/worker"
	dfg "github.com/GTDGit/gtd_api/pkg/digiflazz"
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

	// 6. Initialize services
	authSvc := service.NewAuthService(clientRepo)
	adminAuthSvc := service.NewAdminAuthService(adminRepo)
	clientSvc := service.NewClientService(clientRepo)
	productSvc := service.NewProductService(productRepo, skuRepo)
	productMgmtSvc := service.NewProductManagementService(productRepo, skuRepo)
	callbackSvc := service.NewCallbackService(clientRepo, cbRepo)
	syncSvc := service.NewSyncService(digiProd, productRepo, skuRepo)
	trxSvc := service.NewTransactionService(trxRepo, productRepo, skuRepo, cbRepo, digiProd, digiDev, productSvc, callbackSvc, inquiryCache)

	// 7. Initialize handlers
	handlers := &Handlers{
		Health:            handler.NewHealthHandler(digiProd),
		Product:           handler.NewProductHandler(productSvc),
		Balance:           handler.NewBalanceHandler(digiProd),
		Transaction:       handler.NewTransactionHandler(trxSvc, productSvc),
		Webhook:           handler.NewWebhookHandler(callbackSvc, cfg.Digiflazz.WebhookSecret),
		Client:            handler.NewClientHandler(clientSvc),
		ProductManagement: handler.NewProductManagementHandler(productMgmtSvc),
		Auth:              handler.NewAuthHandler(adminAuthSvc),
		Territory:         handler.NewTerritoryHandler(territoryRepo),
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
	router.Use(middleware.LoggingMiddleware())
	setupRoutes(router, handlers, authMw, jwtMw)

	// 10. Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 11. Start workers
	go worker.NewSyncWorker(syncSvc, cfg.Worker.SyncInterval).Start(ctx)
	go worker.NewRetryWorker(trxSvc, trxRepo, cfg.Worker.RetryInterval).Start(ctx)
	go worker.NewCallbackWorker(callbackSvc, cfg.Worker.CallbackInterval).Start(ctx)

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
	Auth              *handler.AuthHandler
	Territory         *handler.TerritoryHandler
}

// setupRoutes registers all routes.
func setupRoutes(router *gin.Engine, handlers *Handlers, authMiddleware *middleware.AuthMiddleware, jwtMiddleware *middleware.JWTMiddleware) {
	router.POST("/webhook/digiflazz", handlers.Webhook.HandleDigiflazzCallback)
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
		territory.GET("/city/:province_code", handlers.Territory.GetCitiesByProvince)
		territory.GET("/district/:city_code", handlers.Territory.GetDistrictsByCity)
		territory.GET("/sub-district/:district_code", handlers.Territory.GetSubDistrictsByDistrict)
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
