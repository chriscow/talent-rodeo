package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"sync"

	"talent-rodeo/internal/auth"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/handlers"
	"talent-rodeo/internal/handlers/search/providers/scrapingdog"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/tasks"
	th "talent-rodeo/internal/tasks/handlers"
	"talent-rodeo/internal/version"

	"github.com/chriscow/minds/providers/gemini"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/pusher/pusher-http-go/v5"
)

func main() {
	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	err := run(logger)
	if err != nil {
		trace := string(debug.Stack())
		logger.Error(err.Error(), "trace", trace)
		os.Exit(1)
	}
}

type config struct {
	baseURL  string
	httpPort int
	nats     struct {
		port     int
		embedded bool
		url      string
	}
	autoHTTPS struct {
		domain  string
		email   string
		staging bool
	}
	cookie struct {
		secretKey string
	}
	db struct {
		dsn         string
		automigrate bool
	}
	notifications struct {
		email string
	}
	session struct {
		secretKey    string
		oldSecretKey string
	}
	smtp struct {
		host     string
		port     int
		username string
		password string
		from     string
	}
	linkedin struct {
		clientID     string
		clientSecret string
	}
	testing bool // Flag to indicate if the application is running in test mode
}

type application struct {
	config        config
	db            *database.DB
	logger        *slog.Logger
	mailer        *smtp.Mailer
	sessionStore  *sessions.CookieStore
	wg            sync.WaitGroup
	pusher        *pusher.Client
	keywordsMutex sync.RWMutex
	handlers      map[string]any

	natsServer   *server.Server
	natsShutdown chan struct{}

	processor *tasks.Processor
	scheduler *th.NATSMessageScheduler
}

func run(logger *slog.Logger) error {
	if err := checkEnv(); err != nil {
		return err
	}

	var cfg config

	flag.StringVar(&cfg.baseURL, "base-url", os.Getenv("BASE_URL"), "base URL for the application")
	flag.IntVar(&cfg.httpPort, "http-port", 2304, "port to listen on for HTTP requests")
	flag.IntVar(&cfg.nats.port, "nats-port", 42223, "port to listen on for NATS")
	flag.StringVar(&cfg.nats.url, "nats-url", "nats://localhost:42223", "URL for NATS server")
	flag.BoolVar(&cfg.nats.embedded, "embedded-nats", true, "use embedded NATS server")
	flag.StringVar(&cfg.autoHTTPS.domain, "auto-https-domain", "", "domain to enable automatic HTTPS for")
	flag.StringVar(&cfg.autoHTTPS.email, "auto-https-email", "admin@example.com", "contact email address for problems with LetsEncrypt certificates")
	flag.BoolVar(&cfg.autoHTTPS.staging, "auto-https-staging", false, "use LetsEncrypt staging environment")
	flag.StringVar(&cfg.cookie.secretKey, "cookie-secret-key", os.Getenv("SESSION_SECRET"), "secret key for cookie authentication/encryption")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("DB_DSN"), "PostgreSQL DSN")
	flag.BoolVar(&cfg.db.automigrate, "db-automigrate", true, "run migrations on startup")
	flag.StringVar(&cfg.notifications.email, "notifications-email", "", "contact email address for error notifications")
	flag.StringVar(&cfg.session.secretKey, "session-secret-key", os.Getenv("SESSION_SECRET"), "secret key for session cookie authentication")
	flag.StringVar(&cfg.session.oldSecretKey, "session-old-secret-key", "", "previous secret key for session cookie authentication")
	flag.StringVar(&cfg.smtp.host, "smtp-host", "example.smtp.host", "smtp host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 25, "smtp port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", "example_username", "smtp username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", "pa55word", "smtp password")
	flag.StringVar(&cfg.smtp.from, "smtp-from", "Example Name <no-reply@example.org>", "smtp sender")

	flag.StringVar(&cfg.linkedin.clientID, "linkedin-client-id", os.Getenv("LINKEDIN_AUTH_CLIENT_ID"), "LinkedIn client ID")
	flag.StringVar(&cfg.linkedin.clientSecret, "linkedin-client-secret", os.Getenv("LINKEDIN_AUTH_CLIENT_SECRET"), "LinkedIn client secret")

	if cfg.linkedin.clientID == "" || cfg.linkedin.clientSecret == "" {
		return fmt.Errorf("LINKEDIN_AUTH_CLIENT_ID and LINKEDIN_AUTH_CLIENT_SECRET environment variables are required")
	}

	showVersion := flag.Bool("version", false, "display version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	var db *database.DB
	fmt.Println("starting application", "version", version.Get(), "database:", cfg.db.dsn)
	db, err := database.New(cfg.db.dsn, cfg.db.automigrate, database.WithLogger(logger))
	if err != nil {
		return err
	}
	defer db.Close()
	defer func() {
		logger.Debug("closing database connection")
		db.Close()
		logger.Info("database connection closed")
	}()

	mailer, err := smtp.NewMailer(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.from)
	if err != nil {
		return err
	}

	keyPairs := [][]byte{[]byte(cfg.session.secretKey), nil}
	if cfg.session.oldSecretKey != "" {
		keyPairs = append(keyPairs, []byte(cfg.session.oldSecretKey), nil)
	}

	if len(cfg.session.secretKey) < 32 {
		return fmt.Errorf("SESSION_SECRET must be at least 32 bytes long")
	}

	sessionStore := sessions.NewCookieStore(keyPairs...)
	sessionStore.Options = &sessions.Options{
		HttpOnly: true,
		MaxAge:   86400 * 7,
		Path:     "/",
		SameSite: http.SameSiteLaxMode,
		Secure:   true,
	}

	gothic.Store = sessionStore
	goth.UseProviders(
		auth.New(cfg.linkedin.clientID, cfg.linkedin.clientSecret, cfg.baseURL+"/auth/linkedin/callback"),
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create NATS server if embedded mode is enabled
	var natsServer *server.Server
	if cfg.nats.embedded {
		natsServer, err = handlers.StartNATSServer(ctx, handlers.NATSConfig{
			Port:      cfg.nats.port,
			JetStream: true,
		})
		if err != nil {
			return fmt.Errorf("failed to setup NATS: %w", err)
		}
	}

	// Create JetStream context
	nc, err := nats.Connect(cfg.nats.url)
	if err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}
	defer nc.Close()

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Create the main stream
	stream, err := handlers.CreateStream(ctx, cfg.nats.url, logger)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}
	_ = stream // We keep the stream reference even though we don't use it directly

	// Setup all pipeline handlers
	handlers, err := setupPipelineHandlers(ctx, db, js, logger)
	if err != nil {
		return fmt.Errorf("failed to setup pipeline handlers: %w", err)
	}

	pusherClient := setupPusher(logger)

	// This is to schedule delivery of messages for scheduled phone calls
	taskProcessor := tasks.NewProcessor(db, logger)

	// Setup scheduler
	scheduler, err := setupNATSHandler(cfg.nats.url, logger)
	if err != nil {
		return fmt.Errorf("error setting up NATS handler: %w", err)
	}
	taskProcessor.RegisterHandler(scheduler)

	app := &application{
		config:       cfg,
		db:           db,
		logger:       logger,
		mailer:       mailer,
		sessionStore: sessionStore,
		pusher:       &pusherClient,
		natsServer:   natsServer,
		natsShutdown: make(chan struct{}),
		processor:    taskProcessor,
		scheduler:    scheduler,
		handlers:     handlers,
	}

	if cfg.autoHTTPS.domain != "" {
		return app.serveAutoHTTPS()
	}

	return app.serveHTTP()
}

func setupPusher(logger *slog.Logger) pusher.Client {
	logger.Debug("pusher config", "app_id", os.Getenv("PUSHER_APP_ID"), "key",
		os.Getenv("PUSHER_KEY"), "secret", os.Getenv("PUSHER_SECRET"), "cluster", os.Getenv("PUSHER_CLUSTER"))

	pusherClient := pusher.Client{
		AppID:   os.Getenv("PUSHER_APP_ID"),
		Key:     os.Getenv("PUSHER_KEY"),
		Secret:  os.Getenv("PUSHER_SECRET"),
		Cluster: os.Getenv("PUSHER_CLUSTER"),
		Secure:  true,
	}

	return pusherClient
}

// setupNATSHandler sets up the NATS client and returns the handler.
func setupNATSHandler(natsURL string, logger *slog.Logger) (*th.NATSMessageScheduler, error) {
	// Connect to NATS
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	return th.NewNATSMessageScheduler(js, logger), nil
}

func (app *application) setupSessionStore(authKey, encryptKey []byte) {
	store := sessions.NewCookieStore(authKey, encryptKey)

	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7 days
		HttpOnly: true,
		Secure:   false, // set to true in production
		SameSite: http.SameSiteLaxMode,
	}

	app.sessionStore = store
}

func checkEnv() error {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file: %s\n", err.Error())
	}

	requiredKeys := []string{"BASE_URL", "SESSION_SECRET", "SMTP_USERNAME",
		"SMTP_PASSWORD", "LINKEDIN_AUTH_CLIENT_ID", "LINKEDIN_AUTH_CLIENT_SECRET",
		"OPENAI_API_KEY", "PUSHER_APP_ID", "PUSHER_KEY", "PUSHER_SECRET", "PUSHER_CLUSTER"}

	for _, key := range requiredKeys {
		if os.Getenv(key) == "" {
			return fmt.Errorf("%s environment variable is required", key)
		}
	}

	return nil
}

// setupPipelineHandlers configures all pipeline handlers with JetStream
func setupPipelineHandlers(
	ctx context.Context,
	db *database.DB,
	js jetstream.JetStream,
	logger *slog.Logger,
) (map[string]any, error) {
	pipelineHandlers := make(map[string]any)
	logger.Debug("setting up pipeline handlers")

	// Setup voice handler
	llm, err := gemini.NewProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("error setting up gemini provider: %w", err)
	}

	voiceConfig := handlers.CallConfig{
		FromNumber: os.Getenv("TWILIO_FROM_NUMBER"),
		Twiml:      fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><Response><Connect><Stream url="wss://%s/schedule/voice"/></Connect><Start><Stream name="stream"/></Start><Say>Welcome to your scheduled call. Please wait while we connect you.</Say></Response>`, os.Getenv("APP_DOMAIN")),
	}

	// Create voice handler
	voiceHandler, err := handlers.NewVoiceHandler(ctx, js, th.NewRealTwilioClient(
		os.Getenv("TWILIO_ACCOUNT_SID"),
		os.Getenv("TWILIO_AUTH_TOKEN"),
		os.Getenv("TWILIO_FROM_NUMBER"),
	), logger, voiceConfig)
	if err != nil {
		return nil, fmt.Errorf("error setting up voice handler: %w", err)
	}
	pipelineHandlers["voice"] = voiceHandler
	logger.Debug("voice handler configured")

	// Setup transcript handler
	transcriptHandler, err := handlers.NewExperienceHandler(ctx, js, llm, logger)
	if err != nil {
		return nil, fmt.Errorf("error setting up transcript handler: %w", err)
	}
	pipelineHandlers["transcript"] = transcriptHandler
	logger.Debug("transcript handler configured")

	// Setup job description handler
	jobDescHandler, err := handlers.NewRequirementsHandler(ctx, js, llm, logger)
	if err != nil {
		return nil, fmt.Errorf("error setting up job description handler: %w", err)
	}
	pipelineHandlers["jobDesc"] = jobDescHandler
	logger.Debug("job description handler configured")

	// Setup culture handler
	cultureHandler, err := handlers.NewCultureHandler(ctx, js, llm, logger)
	if err != nil {
		return nil, fmt.Errorf("error setting up culture handler: %w", err)
	}
	pipelineHandlers["culture"] = cultureHandler
	logger.Debug("culture handler configured")

	// Setup assessment handler
	assessmentHandler, err := handlers.NewAssessmentHandler(ctx, js, llm, logger, db)
	if err != nil {
		return nil, fmt.Errorf("error setting up assessment handler: %w", err)
	}
	pipelineHandlers["assessment"] = assessmentHandler
	logger.Debug("assessment handler configured")

	// Setup search handler with ScrapingDog provider
	searchProvider := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"))
	searchHandler, err := handlers.NewSearchHandler(ctx, js, searchProvider, logger)
	if err != nil {
		return nil, fmt.Errorf("error setting up search handler: %w", err)
	}
	pipelineHandlers["search"] = searchHandler
	logger.Debug("search handler configured")

	logger.Info("All pipeline handlers configured successfully")
	return pipelineHandlers, nil
}
