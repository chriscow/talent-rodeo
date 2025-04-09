package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"talent-rodeo/cmd/web/search"

	"talent-rodeo/internal"
	"talent-rodeo/internal/auth"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/tasks"
	th "talent-rodeo/internal/tasks/handlers"
	"talent-rodeo/internal/version"

	"cloud.google.com/go/firestore"
	scsfs "github.com/alexedwards/scs/firestore"
	"github.com/alexedwards/scs/v2"
	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
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
	firebase struct {
		projectID string
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
	config         config
	logger         *slog.Logger
	fs             *firestore.Client
	js             jetstream.JetStream
	processor      *tasks.Processor
	search         *search.Service
	wg             sync.WaitGroup
	natsShutdown   chan struct{}
	mailer         *smtp.Mailer
	pusher         *pusher.Client
	sessionManager *scs.SessionManager
}

func run(logger *slog.Logger) error {
	if err := checkEnv(); err != nil {
		return err
	}

	// To initialize Sentry's handler, you need to initialize Sentry itself beforehand
	// https://chris-cowherd.sentry.io/projects/talent-rodeo-dev/getting-started/
	if err := sentry.Init(sentry.ClientOptions{
		Dsn: "https://17ea83f451376a7be4907f9020704128@o4508059028946944.ingest.us.sentry.io/4509140632010752",
	}); err != nil {
		fmt.Printf("Sentry initialization failed: %v\n", err)
		return err
	}

	defer sentry.Flush(time.Second * 5)
	// Capture panics and send them to Sentry
	defer func() {
		if err := recover(); err != nil {
			sentry.CurrentHub().Recover(err)
			sentry.Flush(time.Second * 5)
			fmt.Printf("Panic: %v\n", err)
			fmt.Printf("Stack trace: %s\n", string(debug.Stack()))
		}
	}()

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

	flag.StringVar(&cfg.firebase.projectID, "firebase-project-id", os.Getenv("FIREBASE_PROJECT_ID"), "Firebase project ID")

	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("DB_DSN"), "PostgreSQL DSN")
	flag.BoolVar(&cfg.db.automigrate, "db-automigrate", false, "run migrations on startup")

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

	flag.BoolVar(&cfg.testing, "testing", false, "enable testing mode")

	showVersion := flag.Bool("version", false, "display version and exit")

	flag.Parse()

	if *showVersion {
		fmt.Printf("version: %s\n", version.Get())
		return nil
	}

	// Set testing mode environment variable for components that can't access the flag
	if cfg.testing {
		logger.Warn(internal.BGYellow("===== running in testing mode ====="))
		os.Setenv("TESTING_MODE", "true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firestoreClient, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		return err
	}
	defer firestoreClient.Close()

	/*
		var db *database.DB
		fmt.Println("starting application", "version", version.Get(), "database:", cfg.db.dsn)
		db, err = database.New(cfg.db.dsn, cfg.db.automigrate, database.WithLogger(logger))
		if err != nil {
			return err
		}
		defer db.Close()
		defer func() {
			logger.Debug("app:closing database connection")
			db.Close()
			logger.Info("app: database connection closed")
		}()
	*/

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

	sessionManager := scs.New()
	sessionManager.Store = scsfs.New(firestoreClient)
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Name = "session"
	sessionManager.Cookie.Persist = true
	sessionManager.Cookie.Path = "/"

	// Create and set our adapter as the gothic store
	gothic.Store = auth.NewSCSAdapter(sessionManager)
	goth.UseProviders(
		auth.New(cfg.linkedin.clientID, cfg.linkedin.clientSecret, cfg.baseURL+"/auth/linkedin/callback"),
	)

	// Create NATS server if embedded mode is enabled
	/*
		var natsServer *server.Server
		if cfg.nats.embedded {
			natsServer, err = handlers.StartNATSServer(ctx, server.Options{
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
		_ = stream

		// Setup all pipeline handlers
		handlers, err := setupPipelineHandlers(ctx, db, js, logger)
		if err != nil {
			return fmt.Errorf("failed to setup pipeline handlers: %w", err)
		}
	*/

	pusherClient := setupPusher(logger)

	// This is to schedule delivery of messages for scheduled phone calls
	taskProcessor := tasks.NewProcessor(tasks.NewFirestoreTaskStorage(firestoreClient), logger)

	// Setup scheduler
	// scheduler, err := setupNATSHandler(cfg.nats.url, logger)
	// if err != nil {
	// 	return fmt.Errorf("error setting up NATS handler: %w", err)
	// }
	wh := th.NewWebhookScheduler(logger, firestoreClient)
	// taskProcessor.RegisterHandler(scheduler)
	taskProcessor.RegisterHandler(wh)

	searchProvider := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"),
		scrapingdog.WithFirestore(firestoreClient, logger))

	// Start search workers for ScrapingDog provider
	searchProvider.StartSearchWorkers(ctx, 5) // Start 5 workers to process the search queue

	searchService := search.NewService(firestoreClient, logger, searchProvider, mailer, cfg.notifications.email, pusherClient)
	if err := searchService.Start(); err != nil {
		return fmt.Errorf("failed to start search service: %w", err)
	}

	searchProvider.SetEventHandler(searchService)

	app := &application{
		config: cfg,
		fs:     firestoreClient,
		logger: logger,
		mailer: mailer,
		pusher: pusherClient,
		// natsServer:     natsServer,
		// jetstream:      js,
		// natsShutdown:   make(chan struct{}),
		processor: taskProcessor,
		// natsScheduler: scheduler,
		// handlers:       handlers,
		sessionManager: sessionManager,
		search:         searchService,
	}

	// Start the task processor
	go func() {
		app.logger.Info("app: starting task processor")
		if err := taskProcessor.Start(ctx); err != nil {
			app.logger.Error("task processor error", "error", err)
		}
	}()

	if cfg.autoHTTPS.domain != "" {
		return app.serveAutoHTTPS()
	}

	return app.serveHTTP()
}

func setupPusher(logger *slog.Logger) *pusher.Client {
	logger.Debug("app:pusher config", "app_id", os.Getenv("PUSHER_APP_ID"), "key",
		os.Getenv("PUSHER_KEY"), "secret", os.Getenv("PUSHER_SECRET"), "cluster", os.Getenv("PUSHER_CLUSTER"))

	pusherClient := pusher.Client{
		AppID:   os.Getenv("PUSHER_APP_ID"),
		Key:     os.Getenv("PUSHER_KEY"),
		Secret:  os.Getenv("PUSHER_SECRET"),
		Cluster: os.Getenv("PUSHER_CLUSTER"),
		Secure:  true,
	}

	return &pusherClient
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

func checkEnv() error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	fmt.Println("checkEnv: working directory", internal.BGYellow(wd))

	if err = godotenv.Overload(); err != nil {
		fmt.Printf("checkEnv: Error loading .env file: %s\n", err.Error())
	}

	requiredKeys := []string{"BASE_URL", "SESSION_SECRET", "SMTP_USERNAME",
		"SMTP_PASSWORD", "LINKEDIN_AUTH_CLIENT_ID", "LINKEDIN_AUTH_CLIENT_SECRET",
		"OPENAI_API_KEY", "GEMINI_API_KEY", "PUSHER_APP_ID", "PUSHER_KEY", "PUSHER_SECRET", "PUSHER_CLUSTER",

		"TWILIO_ACCOUNT_SID", "TWILIO_AUTH_TOKEN", "TWILIO_FROM_NUMBER",

		"FIREBASE_PROJECT_ID", "FIREBASE_CREDENTIALS",
		// "CLOUDFLARE_API_EMAIL", "CLOUDFLARE_API_TOKEN",
		// "CLOUDFLARE_GLOBAL_API_KEY", "CLOUDFLARE_KV_NAMESPACE_ID", "SCRAPINGDOG_API_KEY",
		"VAPI_API_KEY", "VAPI_ASSISTANT_ID", "VAPI_OUTBOUND_PHONE_ID",
		"VAPI_WEBHOOK_URL",     // URL that will receive webhooks from VAPI
		"VAPI_START_CALL_PATH", // URL that we will use to start a call (we will POST to this URL to start a call)
	}

	missingKeys := []string{}
	for _, key := range requiredKeys {
		if os.Getenv(key) == "" {
			missingKeys = append(missingKeys, key)
		}
	}

	if len(missingKeys) > 0 {
		slog.Error("Missing required environment variables", "missing_keys", missingKeys)
		return fmt.Errorf("missing required environment variables: %v", missingKeys)
	}

	firebaseCredentials := os.Getenv("FIREBASE_CREDENTIALS")
	if firebaseCredentials == "" {
		return fmt.Errorf("FIREBASE_CREDENTIALS environment variable is required")
	}

	var creds map[string]any
	if err := json.Unmarshal([]byte(firebaseCredentials), &creds); err != nil {
		return fmt.Errorf("failed to unmarshal FIREBASE_CREDENTIALS: %w", err)
	}

	slog.Info("All required environment variables are present")
	return nil
}
