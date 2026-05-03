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
	"github.com/GTDGit/gtd_api/internal/sse"
	"github.com/GTDGit/gtd_api/internal/utils"
	"github.com/GTDGit/gtd_api/internal/worker"
	"github.com/GTDGit/gtd_api/pkg/alterra"
	"github.com/GTDGit/gtd_api/pkg/bnc"
	"github.com/GTDGit/gtd_api/pkg/bri"
	"github.com/GTDGit/gtd_api/pkg/dana"
	dfg "github.com/GTDGit/gtd_api/pkg/digiflazz"
	"github.com/GTDGit/gtd_api/pkg/kiosbank"
	"github.com/GTDGit/gtd_api/pkg/midtrans"
	"github.com/GTDGit/gtd_api/pkg/pakailink"
	"github.com/GTDGit/gtd_api/pkg/xendit"
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
	utils.SetJWTSecret(cfg.JWTSecret)

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

	// 4. Initialize Digiflazz clients (DISABLED - soft-deleted)
	// digiProd := dfg.NewClient(cfg.Digiflazz.Username, cfg.Digiflazz.KeyProduction)
	// digiDev := dfg.NewClient(cfg.Digiflazz.Username, cfg.Digiflazz.KeyDevelopment)
	var digiProd, digiDev *dfg.Client // nil - Digiflazz disabled

	// 5. Initialize repositories
	clientRepo := repository.NewClientRepository(db)
	productRepo := repository.NewProductRepository(db)
	skuRepo := repository.NewSKURepository(db)
	trxRepo := repository.NewTransactionRepository(db)
	cbRepo := repository.NewCallbackRepository(db)
	adminRepo := repository.NewAdminUserRepository(db)
	bankCodeRepo := repository.NewBankCodeRepository(db)
	transferRepo := repository.NewTransferRepository(db)
	ppobProviderRepo := repository.NewPPOBProviderRepository(db)
	paymentRepo := repository.NewPaymentRepository(db)

	// 5a. Initialize PPOB provider clients
	kioskbankProdClient, kioskbankDevClient := buildKiosbankClients(cfg.Kiosbank)

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

	var bncClient *bnc.Client
	if cfg.Disbursement.BNC.ClientID != "" &&
		cfg.Disbursement.BNC.ClientSecret != "" &&
		cfg.Disbursement.BNC.PartnerID != "" &&
		cfg.Disbursement.BNC.ChannelID != "" &&
		cfg.Disbursement.BNC.SourceAccount != "" &&
		cfg.Disbursement.BNC.PrivateKeyPath != "" {
		var err error
		bncClient, err = bnc.NewClient(bnc.Config{
			BaseURL:        cfg.Disbursement.BNC.BaseURL,
			ClientID:       cfg.Disbursement.BNC.ClientID,
			ClientSecret:   cfg.Disbursement.BNC.ClientSecret,
			PartnerID:      cfg.Disbursement.BNC.PartnerID,
			ChannelID:      cfg.Disbursement.BNC.ChannelID,
			SourceAccount:  cfg.Disbursement.BNC.SourceAccount,
			PrivateKeyPath: cfg.Disbursement.BNC.PrivateKeyPath,
		})
		if err != nil {
			log.Warn().Err(err).Msg("BNC disbursement client initialization failed - transfer API will be disabled")
		} else {
			log.Info().Msg("BNC disbursement client registered")
		}
	} else {
		log.Info().Msg("BNC disbursement config incomplete - transfer API will be disabled")
	}

	var briClient *bri.Client
	if cfg.BRI.ClientID != "" && cfg.BRI.ClientSecret != "" {
		var err error
		briClient, err = bri.NewClient(bri.Config{
			BaseURL:        cfg.BRI.BaseURL,
			ClientID:       cfg.BRI.ClientID,
			ClientSecret:   cfg.BRI.ClientSecret,
			PartnerID:      cfg.BRI.PartnerID,
			ChannelID:      cfg.BRI.ChannelID,
			SourceAccount:  cfg.BRI.SourceAccount,
			PrivateKeyPath: cfg.BRI.PrivateKeyPath,
			BRIZZIUsername: cfg.BRI.BRIZZIUsername,
		})
		if err != nil {
			log.Warn().Err(err).Msg("BRI client initialization failed - BRIVA/BRIZZI/transfer BRI will be partially disabled")
		} else {
			log.Info().Msg("BRI client registered")
		}
	} else {
		log.Info().Msg("BRI config incomplete - BRIVA/BRIZZI/transfer BRI will be disabled")
	}

	// 5b. Initialize Payment provider clients (optional per-provider)
	var pakailinkClient *pakailink.Client
	if cfg.Payment.Pakailink.ClientID != "" && cfg.Payment.Pakailink.ClientSecret != "" &&
		(cfg.Payment.Pakailink.PrivateKeyPath != "" || cfg.Payment.Pakailink.PrivateKeyPEM != "") {
		var err error
		pakailinkClient, err = pakailink.NewClient(pakailink.Config{
			BaseURL:        cfg.Payment.Pakailink.BaseURL,
			ClientID:       cfg.Payment.Pakailink.ClientID,
			ClientSecret:   cfg.Payment.Pakailink.ClientSecret,
			PartnerID:      cfg.Payment.Pakailink.PartnerID,
			ChannelID:      cfg.Payment.Pakailink.ChannelID,
			PrivateKeyPath: cfg.Payment.Pakailink.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Payment.Pakailink.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Pakailink client initialization failed - VA/QRIS via Pakailink disabled")
			pakailinkClient = nil
		} else {
			log.Info().Msg("Pakailink client registered")
		}
	} else {
		log.Info().Msg("Pakailink config incomplete - VA/QRIS via Pakailink disabled")
	}

	var danaClient *dana.Client
	if cfg.Payment.Dana.ClientID != "" && cfg.Payment.Dana.ClientSecret != "" &&
		cfg.Payment.Dana.MerchantID != "" &&
		(cfg.Payment.Dana.PrivateKeyPath != "" || cfg.Payment.Dana.PrivateKeyPEM != "") {
		var err error
		danaClient, err = dana.NewClient(dana.Config{
			BaseURL:        cfg.Payment.Dana.BaseURL,
			MerchantID:     cfg.Payment.Dana.MerchantID,
			ClientID:       cfg.Payment.Dana.ClientID,
			ClientSecret:   cfg.Payment.Dana.ClientSecret,
			PartnerID:      cfg.Payment.Dana.PartnerID,
			PrivateKeyPath: cfg.Payment.Dana.PrivateKeyPath,
			PrivateKeyPEM:  cfg.Payment.Dana.PrivateKeyPEM,
		})
		if err != nil {
			log.Warn().Err(err).Msg("DANA client initialization failed - DANA e-wallet disabled")
			danaClient = nil
		} else {
			log.Info().Msg("DANA client registered")
		}
	} else {
		log.Info().Msg("DANA config incomplete - DANA e-wallet disabled")
	}

	var midtransClient *midtrans.Client
	if cfg.Payment.Midtrans.ServerKey != "" {
		var err error
		midtransClient, err = midtrans.NewClient(midtrans.Config{
			BaseURL:    cfg.Payment.Midtrans.BaseURL,
			ServerKey:  cfg.Payment.Midtrans.ServerKey,
			ClientKey:  cfg.Payment.Midtrans.ClientKey,
			MerchantID: cfg.Payment.Midtrans.MerchantID,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Midtrans client initialization failed - GoPay/ShopeePay disabled")
			midtransClient = nil
		} else {
			log.Info().Msg("Midtrans client registered")
		}
	} else {
		log.Info().Msg("Midtrans config incomplete - GoPay/ShopeePay disabled")
	}

	var xenditClient *xendit.Client
	if cfg.Payment.Xendit.APIKey != "" {
		var err error
		xenditClient, err = xendit.NewClient(xendit.Config{
			BaseURL:      cfg.Payment.Xendit.BaseURL,
			APIKey:       cfg.Payment.Xendit.APIKey,
			APIVersion:   cfg.Payment.Xendit.APIVersion,
			WebhookToken: cfg.Payment.Xendit.WebhookToken,
		})
		if err != nil {
			log.Warn().Err(err).Msg("Xendit client initialization failed - Indomaret/Alfamart disabled")
			xenditClient = nil
		} else {
			log.Info().Msg("Xendit client registered")
		}
	} else {
		log.Info().Msg("Xendit config incomplete - Indomaret/Alfamart disabled")
	}

	// 6. Initialize services
	authSvc := service.NewAuthService(clientRepo)
	adminAuthSvc := service.NewAdminAuthService(adminRepo)
	clientSvc := service.NewClientService(clientRepo)
	productSvc := service.NewProductService(productRepo, skuRepo)
	productMasterRepo := repository.NewProductMasterRepository(db)
	productMasterSvc := service.NewProductMasterService(productMasterRepo)
	productMgmtSvc := service.NewProductManagementService(productRepo, skuRepo, productMasterSvc)
	callbackSvc := service.NewCallbackService(clientRepo, cbRepo, trxRepo)
	// syncSvc disabled - Digiflazz sync no longer needed
	_ = service.NewSyncService // keep import alive
	trxSvc := service.NewTransactionService(trxRepo, productRepo, skuRepo, cbRepo, digiProd, digiDev, productSvc, callbackSvc, inquiryCache)

	// Wire up callback service to transaction service for immediate retry on webhook
	callbackSvc.SetTransactionRetrier(trxSvc)

	// Initialize SSE hub and notifier for real-time admin updates
	sseHub := sse.NewHub()
	sseNotifier := sse.NewHubNotifier(sseHub)
	trxSvc.SetNotifier(sseNotifier)
	callbackSvc.SetNotifier(sseNotifier)

	// Initialize Admin Transaction service
	adminTrxSvc := service.NewAdminTransactionService(trxRepo, cbRepo, productSvc, trxSvc, callbackSvc)

	// Initialize Provider Router for multi-provider PPOB
	providerRouter := service.NewProviderRouter(ppobProviderRepo)
	if kioskbankProdClient != nil {
		kiosbankAdapter := service.NewKiosbankProviderClient(kioskbankProdClient, kioskbankDevClient, trxRepo, cbRepo, ppobProviderRepo)
		providerRouter.RegisterProvider(models.ProviderKiosbank, kiosbankAdapter)
		log.Info().Msg("Kiosbank provider registered")
	}
	if alterraClient != nil {
		alterraAdapter := service.NewAlterraProviderClient(alterraClient, alterraClient) // Same client for prod/dev
		providerRouter.RegisterProvider(models.ProviderAlterra, alterraAdapter)
		log.Info().Msg("Alterra provider registered")
	}
	if briClient != nil && cfg.BRI.BRIZZIUsername != "" {
		briAdapter := service.NewBRIProviderClient(briClient, cfg.BRI.BRIZZIDenominations)
		providerRouter.RegisterProvider(models.ProviderBRI, briAdapter)
		log.Info().Msg("BRI provider registered for BRIZZI")
	}
	// Digiflazz disabled - soft-deleted from providers
	// digiAdapter := service.NewDigiflazzProviderClient(digiProd, digiDev)
	// providerRouter.RegisterProvider(models.ProviderDigiflazz, digiAdapter)
	// log.Info().Msg("Digiflazz provider registered (backup)")

	// Wire provider router to transaction service for multi-provider support
	trxSvc.SetProviderRouter(providerRouter)
	log.Info().Msg("Provider router connected to transaction service")

	// Update product service with provider-aware version for best price
	productSvc = service.NewProductServiceWithProviders(productRepo, skuRepo, ppobProviderRepo)

	// Initialize provider callback service
	providerCallbackSvc := service.NewProviderCallbackService(ppobProviderRepo, trxRepo, callbackSvc)
	providerCallbackSvc.SetNotifier(sseNotifier)
	providerCallbackSvc.SetRetrier(trxSvc)
	transferCallbackSvc := service.NewTransferCallbackService(clientRepo, bankCodeRepo)
	transferSvc := service.NewTransferService(
		transferRepo,
		bankCodeRepo,
		bncClient,
		briClient,
		transferCallbackSvc,
		cfg.Disbursement.BNC.SourceAccount,
		cfg.BRI.SourceAccount,
	)
	if pakailinkClient != nil && cfg.Disbursement.Pakailink.Enabled {
		transferSvc.SetPakailinkClient(
			service.NewPakailinkTransferAdapter(pakailinkClient, cfg.Disbursement.Pakailink.CallbackURL),
			cfg.Disbursement.Pakailink.SourceLabel,
		)
		log.Info().Msg("PakaiLink disbursement adapter registered (handles all banks)")
	}
	bncConnectorSvc := service.NewBNCConnectorService(
		transferRepo,
		transferSvc,
		cfg.JWTSecret,
		cfg.Disbursement.BNC.ClientSecret,
		cfg.Disbursement.BNC.ConnectorClientKey,
		cfg.Disbursement.BNC.ConnectorPublicKeyPEM,
		cfg.Disbursement.BNC.ConnectorPublicKeyPath,
		cfg.Disbursement.BNC.Env,
	)
	briConnectorClientKey := cfg.BRI.ConnectorClientKey
	if briConnectorClientKey == "" {
		briConnectorClientKey = cfg.BRI.ClientID
	}
	briConnectorSvc := service.NewBRIConnectorService(
		paymentRepo,
		cfg.JWTSecret,
		cfg.BRI.ClientSecret,
		briConnectorClientKey,
		cfg.BRI.ConnectorPublicKeyPEM,
		cfg.BRI.ConnectorPublicKeyPath,
		cfg.BRI.Env,
	)

	// 6b. Payment module wiring
	paymentRouter := service.NewPaymentProviderRouter()
	if pakailinkClient != nil {
		paymentRouter.Register(service.NewPakailinkProviderClient(pakailinkClient, cfg.Payment.Pakailink.CallbackURL))
		log.Info().Msg("Pakailink payment adapter registered")
	}
	if danaClient != nil {
		paymentRouter.Register(service.NewDanaProviderClient(danaClient, cfg.Payment.Dana.CallbackURL, cfg.Payment.Dana.ReturnURL))
		log.Info().Msg("DANA payment adapter registered")
	}
	if midtransClient != nil {
		paymentRouter.Register(service.NewMidtransProviderClient(midtransClient, cfg.Payment.Midtrans.CallbackURL))
		log.Info().Msg("Midtrans payment adapter registered")
	}
	if xenditClient != nil {
		paymentRouter.Register(service.NewXenditProviderClient(xenditClient))
		log.Info().Msg("Xendit payment adapter registered")
	}

	paymentCallbackSvc := service.NewPaymentCallbackService(paymentRepo, clientRepo)
	paymentSvc := service.NewPaymentService(paymentRepo, clientRepo, paymentRouter, paymentCallbackSvc)
	paymentSvc.SetNotifier(sseNotifier)
	adminPaymentSvc := service.NewAdminPaymentService(paymentRepo, clientRepo, paymentSvc, paymentCallbackSvc)

	// Resolve webhook secrets for inbound signature verification.
	pakailinkWebhookSecret := cfg.Payment.Pakailink.ClientSecret
	if pakailinkClient != nil {
		pakailinkWebhookSecret = pakailinkClient.ClientSecret()
	}
	danaWebhookSecret := cfg.Payment.Dana.ClientSecret
	midtransWebhookSecret := cfg.Payment.Midtrans.ServerKey
	if cfg.Payment.Midtrans.WebhookSecret != "" {
		midtransWebhookSecret = cfg.Payment.Midtrans.WebhookSecret
	}
	xenditWebhookToken := cfg.Payment.Xendit.WebhookToken

	// 7. Initialize handlers
	handlers := &Handlers{
		Health:            handler.NewHealthHandler(digiProd),
		Product:           handler.NewProductHandler(productSvc),
		Balance:           handler.NewBalanceHandler(digiProd),
		Transaction:       handler.NewTransactionHandler(trxSvc, productSvc),
		Webhook:           handler.NewWebhookHandler(callbackSvc, cfg.Digiflazz.WebhookSecret),
		Client:            handler.NewClientHandler(clientSvc),
		ProductManagement: handler.NewProductManagementHandler(productMgmtSvc),
		ProductMaster:     handler.NewProductMasterHandler(productMasterSvc),
		AdminTransaction:  handler.NewAdminTransactionHandler(adminTrxSvc),
		Auth:              handler.NewAuthHandler(adminAuthSvc),
		BankCode:          handler.NewBankCodeHandler(bankCodeRepo),
		Transfer:          handler.NewTransferHandler(transferSvc),
		BNCConnector:      handler.NewBNCConnectorHandler(bncConnectorSvc),
		BRIConnector:      handler.NewBRIConnectorHandler(briConnectorSvc),
		PPOBProvider:      handler.NewPPOBProviderHandler(ppobProviderRepo),
		ProviderCallback:  handler.NewProviderCallbackHandler(providerCallbackSvc, cfg.Alterra.CallbackPublicKey),
		SSE:               handler.NewSSEHandler(sseHub),
		Payment:           handler.NewPaymentHandler(paymentSvc),
		PaymentWebhook: handler.NewPaymentWebhookHandler(
			paymentRepo,
			paymentSvc,
			pakailinkWebhookSecret,
			danaWebhookSecret,
			midtransWebhookSecret,
			xenditWebhookToken,
		),
		DisbursementWebhook: handler.NewDisbursementWebhookHandler(
			transferRepo,
			transferSvc,
			pakailinkWebhookSecret,
		),
		AdminPayment: handler.NewAdminPaymentHandler(adminPaymentSvc),
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
	// Digiflazz sync worker disabled - no longer syncing from Digiflazz
	// go worker.NewSyncWorker(syncSvc, cfg.Worker.SyncInterval).Start(ctx)
	go worker.NewRetryWorker(trxRepo, callbackSvc, cfg.Worker.RetryInterval).Start(ctx)
	go worker.NewCallbackWorker(callbackSvc, cfg.Worker.CallbackInterval).Start(ctx)
	// Digiflazz callback worker disabled
	// go worker.NewDigiflazzCallbackWorker(cbRepo, trxRepo, trxSvc, callbackSvc, cfg.Worker.DigiflazzCallbackInterval).Start(ctx)
	go worker.NewStatusCheckWorker(
		trxRepo, skuRepo, callbackSvc, digiProd, digiDev, providerRouter, trxSvc,
		cfg.Worker.StatusCheckInterval,
		cfg.Worker.StatusCheckStaleAfter,
		cfg.Worker.StatusCheckMaxAge,
		cfg.Kiosbank.StatusCheckMinAge,
		cfg.Kiosbank.StatusCheckMaxAge,
	).Start(ctx)
	go worker.NewTransferStatusWorker(
		transferSvc,
		cfg.Worker.StatusCheckInterval,
		cfg.Worker.StatusCheckStaleAfter,
		cfg.Worker.StatusCheckMaxAge,
		50,
	).Start(ctx)

	// Start provider price sync worker
	providerClients := providerRouter.GetClients()
	go worker.NewProviderSyncWorker(ppobProviderRepo, providerClients, cfg.Worker.SyncInterval).Start(ctx)

	// Payment module workers
	go worker.NewPaymentStatusWorker(
		paymentSvc,
		cfg.Worker.PaymentStatusInterval,
		cfg.Worker.PaymentStatusStaleAfter,
		50,
	).Start(ctx)
	go worker.NewPaymentExpiryWorker(paymentSvc, cfg.Worker.PaymentExpiryInterval, 100).Start(ctx)
	go worker.NewPaymentCallbackWorker(paymentCallbackSvc, cfg.Worker.PaymentCallbackInterval, 50).Start(ctx)

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
	ProductMaster     *handler.ProductMasterHandler
	AdminTransaction  *handler.AdminTransactionHandler
	Auth              *handler.AuthHandler
	BankCode          *handler.BankCodeHandler
	Transfer          *handler.TransferHandler
	BNCConnector      *handler.BNCConnectorHandler
	BRIConnector      *handler.BRIConnectorHandler
	PPOBProvider      *handler.PPOBProviderHandler
	ProviderCallback  *handler.ProviderCallbackHandler
	SSE               *handler.SSEHandler
	Payment              *handler.PaymentHandler
	PaymentWebhook       *handler.PaymentWebhookHandler
	DisbursementWebhook  *handler.DisbursementWebhookHandler
	AdminPayment         *handler.AdminPaymentHandler
}

// setupRoutes registers all routes.
func setupRoutes(router *gin.Engine, handlers *Handlers, authMiddleware *middleware.AuthMiddleware, jwtMiddleware *middleware.JWTMiddleware) {
	// Provider webhook endpoints
	router.POST("/v1/webhook/digiflazz", handlers.Webhook.HandleDigiflazzCallback)
	router.POST("/v1/webhook/kiosbank", handlers.ProviderCallback.HandleKiosbankCallback)
	router.POST("/v1/webhook/alterra", handlers.ProviderCallback.HandleAlterraCallback)
	router.POST("/bnc/v1.0/access-token/b2b", handlers.BNCConnector.CreateAccessToken)
	router.POST("/bnc/v1.0/transfer/notify", handlers.BNCConnector.HandleTransferNotify)
	router.POST("/snap/v1.0/access-token/b2b", handlers.BRIConnector.CreateAccessToken)
	router.POST("/snap/v1.0/transfer-va/notify-payment-intrabank", handlers.BRIConnector.HandleVAPaymentNotify)

	// Payment provider webhooks (public — each handler verifies its own signature).
	router.POST("/v1/webhook/pakailink", handlers.PaymentWebhook.HandlePakailink)
	router.POST("/v1/webhook/dana", handlers.PaymentWebhook.HandleDANA)
	router.POST("/v1/webhook/midtrans", handlers.PaymentWebhook.HandleMidtrans)
	router.POST("/v1/webhook/xendit", handlers.PaymentWebhook.HandleXendit)

	// Disbursement provider webhooks (public — each handler verifies its own signature).
	router.POST("/v1/webhook/pakailink-disbursement", handlers.DisbursementWebhook.HandlePakailink)

	router.GET("/v1/health", handlers.Health.GetHealth)
	// API PPOB routes (protected with client API key + ppob scope)
	ppob := router.Group("/v1/ppob")
	ppob.Use(authMiddleware.Handle(), middleware.RequireScope(middleware.ScopePPOB))
	{
		ppob.GET("/products", handlers.Product.GetProducts)
		ppob.GET("/balance", handlers.Balance.GetBalance)
		ppob.POST("/transaction", handlers.Transaction.CreateTransaction)
		ppob.GET("/transaction/:transactionId", handlers.Transaction.GetTransaction)
	}

	// Bank codes (protected with client API key + disbursement scope)
	router.GET("/v1/bank-codes", authMiddleware.Handle(), middleware.RequireScope(middleware.ScopeDisbursement), handlers.BankCode.GetBankCodes)

	transfer := router.Group("/v1/transfer")
	transfer.Use(authMiddleware.Handle(), middleware.RequireScope(middleware.ScopeDisbursement))
	{
		transfer.POST("/inquiry", handlers.Transfer.CreateInquiry)
		transfer.POST("", handlers.Transfer.CreateTransfer)
		transfer.GET("/:transferId", handlers.Transfer.GetTransfer)
	}

	// Payment client API (protected with client API key + payment scope).
	payment := router.Group("/v1/payment")
	payment.Use(authMiddleware.Handle(), middleware.RequireScope(middleware.ScopePayment))
	{
		payment.GET("/methods", handlers.Payment.ListMethods)
		payment.POST("/create", handlers.Payment.CreatePayment)
		payment.GET("/:paymentId", handlers.Payment.GetPayment)
		payment.POST("/:paymentId/cancel", handlers.Payment.CancelPayment)
		payment.POST("/:paymentId/refund", handlers.Payment.RefundPayment)
	}

	// Admin routes
	admin := router.Group("/v1/admin")
	admin.POST("/auth/login", handlers.Auth.Login)
	admin.GET("/sse", handlers.SSE.Stream) // JWT via ?token= query param (EventSource can't set headers)
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
		admin.GET("/products/variants", handlers.ProductManagement.GetVariants)
		admin.POST("/products", handlers.ProductManagement.CreateProduct)
		admin.GET("/products/:id", handlers.ProductManagement.GetProduct)
		admin.PUT("/products/:id", handlers.ProductManagement.UpdateProduct)
		admin.DELETE("/products/:id", handlers.ProductManagement.DeleteProduct)

		// Product Master (categories, brands, variants) CRUD
		admin.GET("/product-master/categories", handlers.ProductMaster.ListCategories)
		admin.POST("/product-master/categories", handlers.ProductMaster.CreateCategory)
		admin.PUT("/product-master/categories/:id", handlers.ProductMaster.UpdateCategory)
		admin.DELETE("/product-master/categories/:id", handlers.ProductMaster.DeleteCategory)
		admin.GET("/product-master/brands", handlers.ProductMaster.ListBrands)
		admin.POST("/product-master/brands", handlers.ProductMaster.CreateBrand)
		admin.PUT("/product-master/brands/:id", handlers.ProductMaster.UpdateBrand)
		admin.DELETE("/product-master/brands/:id", handlers.ProductMaster.DeleteBrand)
		admin.GET("/product-master/variants", handlers.ProductMaster.ListVariants)
		admin.POST("/product-master/variants", handlers.ProductMaster.CreateVariant)
		admin.PUT("/product-master/variants/:id", handlers.ProductMaster.UpdateVariant)
		admin.DELETE("/product-master/variants/:id", handlers.ProductMaster.DeleteVariant)

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

		// Payment admin
		admin.GET("/payments", handlers.AdminPayment.ListPayments)
		admin.GET("/payments/stats", handlers.AdminPayment.Stats)
		admin.GET("/payments/:id", handlers.AdminPayment.GetPayment)
		admin.GET("/payments/:id/logs", handlers.AdminPayment.GetPaymentLogs)
		admin.GET("/payments/:id/callbacks", handlers.AdminPayment.GetPaymentCallbacks)
		admin.GET("/payments/:id/callback-logs", handlers.AdminPayment.ListCallbackLogs)
		admin.GET("/payments/:id/refunds", handlers.AdminPayment.ListRefunds)
		admin.POST("/payments/:id/retry-callback", handlers.AdminPayment.RetryCallback)
		admin.POST("/payments/:id/refund", handlers.AdminPayment.Refund)

		// Payment method admin
		admin.GET("/payment-methods", handlers.AdminPayment.ListMethods)
		admin.PUT("/payment-methods/:id", handlers.AdminPayment.UpdateMethod)
	}
}

func buildKiosbankClients(cfg config.KiosbankConfig) (*kiosbank.Client, *kiosbank.Client) {
	if cfg.Username == "" {
		return nil, nil
	}
	if cfg.MerchantName == "" {
		log.Warn().Msg("KIOSBANK_MERCHANT_NAME is empty; falling back to KIOSBANK_MERCHANT_ID for sign-on")
	}
	if cfg.DevelopmentURL != "" && cfg.DevelopmentCreds.MerchantName == "" {
		log.Warn().Msg("KIOSBANK_DEV_MERCHANT_NAME is empty; falling back to development merchant ID for sign-on")
	}

	prodClient := kiosbank.NewClient(kiosbankClientConfig(cfg, false))
	devClient := kiosbank.NewClient(kiosbankClientConfig(cfg, true))

	return prodClient, devClient
}

func kiosbankClientConfig(cfg config.KiosbankConfig, development bool) kiosbank.Config {
	if !development {
		merchantName := cfg.MerchantName
		if merchantName == "" {
			merchantName = cfg.MerchantID
		}
		return kiosbank.Config{
			BaseURL:            cfg.BaseURL,
			MerchantID:         cfg.MerchantID,
			MerchantName:       merchantName,
			CounterID:          cfg.CounterID,
			AccountID:          cfg.AccountID,
			Mitra:              cfg.Mitra,
			Username:           cfg.Username,
			Password:           cfg.Password,
			InsecureSkipVerify: cfg.InsecureSkipVerify,
		}
	}

	devCreds := cfg.DevelopmentCreds
	if devCreds.Username == "" {
		devCreds.Username = cfg.Username
		devCreds.Password = cfg.Password
		devCreds.MerchantID = cfg.MerchantID
		devCreds.MerchantName = cfg.MerchantName
		devCreds.CounterID = cfg.CounterID
		devCreds.AccountID = cfg.AccountID
		devCreds.Mitra = cfg.Mitra
	}
	if devCreds.MerchantName == "" {
		devCreds.MerchantName = devCreds.MerchantID
	}

	return kiosbank.Config{
		BaseURL:            cfg.DevelopmentURL,
		MerchantID:         devCreds.MerchantID,
		MerchantName:       devCreds.MerchantName,
		CounterID:          devCreds.CounterID,
		AccountID:          devCreds.AccountID,
		Mitra:              devCreds.Mitra,
		Username:           devCreds.Username,
		Password:           devCreds.Password,
		InsecureSkipVerify: cfg.DevelopmentInsecureSkipVerify,
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
