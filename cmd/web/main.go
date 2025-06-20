package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"talent-rodeo/internal"
	"talent-rodeo/internal/auth"
	"talent-rodeo/internal/feedback"
	"talent-rodeo/internal/nudge"
	"talent-rodeo/internal/search"
	"talent-rodeo/internal/search/providers/scrapingdog"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/subscription"
	"talent-rodeo/internal/tasks"
	th "talent-rodeo/internal/tasks/handlers"
	"talent-rodeo/internal/transcript"
	"talent-rodeo/internal/version"

	scsfs "github.com/alexedwards/scs/firestore"
	"github.com/alexedwards/scs/v2"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/getsentry/sentry-go"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
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

// loadTestUser loads a predefined user from Firestore for testing purposes.
// This is used only when TESTING_MODE=true to provide an authenticated user
// context for UI testing without requiring a login flow.
func (app *application) loadTestUser() (*internal.User, error) {
	// Set a consistent test user ID or use an environment variable if provided
	testUserID := os.Getenv("LINKEDIN_USERNAME")

	// If no test user ID is specified, try to find any user
	if testUserID == "" {
		return nil, fmt.Errorf("no test user ID specified in environment variable LINKEDIN_USERNAME")
	}

	// Load the user by ID
	user, found, err := internal.GetUserByEmail(context.Background(), app.fs, testUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get test user: %w", err)
	}

	if !found {
		return nil, fmt.Errorf("test user not found with ID: %s", testUserID)
	}

	app.logger.Info("loaded test user for testing mode",
		"user_id", user.ID,
		"email", user.Email,
		"status", user.State.Status)

	return user, nil
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
	testing bool   // Flag to indicate if the application is running in test mode
	envFile string // override which env file to load, e.g. ".env.prod" or ".env.dev"
}

type application struct {
	config                config
	logger                *slog.Logger
	fs                    *internal.FirestoreClient
	js                    jetstream.JetStream
	s3                    *s3.S3
	processor             *tasks.Processor
	wg                    sync.WaitGroup
	natsShutdown          chan struct{}
	mailer                smtp.Mailer
	pusher                *pusher.Client
	jobDispatch           *internal.JobEventDispatch
	feedbackProcessor     *feedback.Processor
	searchResultProcessor *internal.SearchResultProcessor
	sessionManager        *scs.SessionManager
	searchProvider        *scrapingdog.Provider
	searchScheduler       *search.Scheduler
	subscriptionProcessor *subscription.Processor
	nudgeProcessor        *nudge.Processor
}

func run(logger *slog.Logger) error {
	// Define ALL flags at once, before any Parse() call
	var cfg config
	flag.StringVar(&cfg.envFile, "env", ".env", "override which env file to load, e.g. '.env.prod' or '.env.dev'")
	flag.IntVar(&cfg.httpPort, "http-port", 2304, "port to listen on for HTTP requests")

	// Define all other flags here (before parsing environment)
	flag.StringVar(&cfg.baseURL, "base-url", "", "base URL for the application") // Note: empty default initially
	flag.IntVar(&cfg.nats.port, "nats-port", 42223, "port to listen on for NATS")
	flag.StringVar(&cfg.nats.url, "nats-url", "nats://localhost:42223", "URL for NATS server")
	flag.BoolVar(&cfg.nats.embedded, "embedded-nats", true, "use embedded NATS server")
	flag.StringVar(&cfg.autoHTTPS.domain, "auto-https-domain", "", "domain to enable automatic HTTPS for")
	flag.StringVar(&cfg.autoHTTPS.email, "auto-https-email", "admin@example.com", "contact email address for problems with LetsEncrypt certificates")
	flag.BoolVar(&cfg.autoHTTPS.staging, "auto-https-staging", false, "use LetsEncrypt staging environment")
	flag.StringVar(&cfg.cookie.secretKey, "cookie-secret-key", "", "secret key for cookie authentication/encryption")
	flag.StringVar(&cfg.firebase.projectID, "firebase-project-id", "", "Firebase project ID")
	flag.StringVar(&cfg.db.dsn, "db-dsn", "", "PostgreSQL DSN")
	flag.BoolVar(&cfg.db.automigrate, "db-automigrate", false, "run migrations on startup")
	flag.StringVar(&cfg.notifications.email, "notifications-email", "", "contact email address for error notifications")
	flag.StringVar(&cfg.session.secretKey, "session-secret-key", "", "secret key for session cookie authentication")
	flag.StringVar(&cfg.session.oldSecretKey, "session-old-secret-key", "", "previous secret key for session cookie authentication")

	flag.StringVar(&cfg.smtp.host, "smtp-host", "smtp.gmail.com", "smtp host")
	flag.IntVar(&cfg.smtp.port, "smtp-port", 587, "smtp port")
	flag.StringVar(&cfg.smtp.username, "smtp-username", "", "smtp username")
	flag.StringVar(&cfg.smtp.password, "smtp-password", "", "smtp password")
	flag.StringVar(&cfg.smtp.from, "smtp-from", "Shirely Hunter <shirely@talent.rodeo>", "smtp sender")

	flag.StringVar(&cfg.linkedin.clientID, "linkedin-client-id", "", "LinkedIn client ID")
	flag.StringVar(&cfg.linkedin.clientSecret, "linkedin-client-secret", "", "LinkedIn client secret")
	flag.BoolVar(&cfg.testing, "testing", false, "enable testing mode")
	showVersion := flag.Bool("version", false, "display version and exit")

	// Parse flags ONCE
	flag.Parse()

	if err := nudge.LoadAndCacheTemplates(); err != nil {
		return fmt.Errorf("failed to load nudge templates: %w", err)
	}

	// Load environment file
	if err := checkEnv(logger, cfg.envFile); err != nil {
		return fmt.Errorf("error checking environment: %w", err)
	}

	// Apply environment variable defaults to config fields that weren't set by flags
	if cfg.baseURL == "" {
		cfg.baseURL = os.Getenv("BASE_URL")
	}
	if cfg.cookie.secretKey == "" {
		cfg.cookie.secretKey = os.Getenv("SESSION_SECRET")
	}
	if cfg.firebase.projectID == "" {
		cfg.firebase.projectID = os.Getenv("FIREBASE_PROJECT_ID")
	}
	if cfg.db.dsn == "" {
		cfg.db.dsn = os.Getenv("DB_DSN")
	}
	if cfg.session.secretKey == "" {
		cfg.session.secretKey = os.Getenv("SESSION_SECRET")
	}

	if cfg.smtp.username == "" {
		cfg.smtp.username = os.Getenv("SMTP_USERNAME")
	}
	if cfg.smtp.password == "" {
		cfg.smtp.password = os.Getenv("SMTP_PASSWORD")
	}

	if cfg.linkedin.clientID == "" {
		cfg.linkedin.clientID = os.Getenv("LINKEDIN_AUTH_CLIENT_ID")
	}
	if cfg.linkedin.clientSecret == "" {
		cfg.linkedin.clientSecret = os.Getenv("LINKEDIN_AUTH_CLIENT_SECRET")
	}

	if cfg.linkedin.clientID == "" || cfg.linkedin.clientSecret == "" {
		return fmt.Errorf("LINKEDIN_AUTH_CLIENT_ID and LINKEDIN_AUTH_CLIENT_SECRET environment variables are required")
	}

	sentryDSN := os.Getenv("SENTRY_DSN")
	if sentryDSN == "" {
		return fmt.Errorf("SENTRY_DSN environment variable is required")
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn: sentryDSN,
	})
	if err != nil {
		log.Fatalf("sentry.Init: %s", err)
	}

	logger.Info("Starting HTTP server", "port", cfg.httpPort)

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

	FirestoreClient, err := internal.NewFirestoreClient(ctx)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "firestore_client_new",
		})
		return fmt.Errorf("failed to create firestore client: %w", err)
	}
	defer FirestoreClient.Close()

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

	mailer, err := smtp.NewMailSender(cfg.smtp.host, cfg.smtp.port, cfg.smtp.username, cfg.smtp.password, cfg.smtp.from)
	if err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "smtp_mailer_new",
		})
		return fmt.Errorf("failed to create new mailer: %w", err)
	}

	sessionManager, err := setupSessionManager(cfg, FirestoreClient, logger)
	if err != nil {
		return fmt.Errorf("failed to setup session manager: %w", err)
	}

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
	taskProcessor := tasks.NewProcessor(tasks.NewFirestoreTaskStorage(FirestoreClient), logger)

	// Setup scheduler
	// scheduler, err := setupNATSHandler(cfg.nats.url, logger)
	// if err != nil {
	// 	return fmt.Errorf("error setting up NATS handler: %w", err)
	// }
	wh := th.NewWebhookScheduler(logger, FirestoreClient)
	// taskProcessor.RegisterHandler(scheduler)
	taskProcessor.RegisterHandler(wh)

	// jobSink := internal.NewJobSink(FirestoreClient, logger)
	// searchProvider := scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"), scrapingdog.JobEventHandler(jobSink), logger)
	// searchScheduler := search.NewSearchScheduler(FirestoreClient, logger, searchProvider, mailer, cfg.notifications.email)

	// searchProvider.Start(ctx)
	// if err := searchScheduler.Start(ctx); err != nil {
	// 	return fmt.Errorf("failed to start search scheduler: %w", err)
	// }
	// if err := jobProcessor.Start(ctx); err != nil {
	// 	return fmt.Errorf("failed to start job processor: %w", err)
	// }

	s3Client, err := initS3Client()
	if err != nil {
		return fmt.Errorf("failed to initialize S3 client: %w", err)
	}

	app := &application{
		config: cfg,
		fs:     FirestoreClient,
		s3:     s3Client,
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
		// search:          searchScheduler,
		// searchProvider:  searchProvider,
		// searchScheduler: searchScheduler,
		// jobProcessor:    jobProcessor,
	}

	// Start the task processor
	go func() {
		app.logger.Info("app: starting task processor")
		if err := taskProcessor.Start(ctx); err != nil {
			app.logger.Error("task processor error", "error", err)
		}
	}()

	smtpCfg := smtp.Config{
		Host:     app.config.smtp.host,
		Port:     app.config.smtp.port,
		Username: app.config.smtp.username,
		Password: app.config.smtp.password,
		From:     app.config.smtp.from,
	}
	if err := app.startServices(ctx, &smtpCfg); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	if err := transcript.RecoverProcessingAppointments(sentry.CurrentHub(), FirestoreClient, pusherClient, logger, app.jobDispatch, app.searchProvider, &smtpCfg); err != nil {
		internal.SentryReportError(ctx, err, sentry.LevelError, map[string]string{
			"error_type": "transcript_recovery",
		})
		return fmt.Errorf("failed to recover processing appointments: %w", err)
	}

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
// func setupNATSHandler(natsURL string, logger *slog.Logger) (*th.NATSMessageScheduler, error) {
// 	// Connect to NATS
// 	nc, err := nats.Connect(natsURL)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
// 	}

// 	// Create JetStream context
// 	js, err := jetstream.New(nc)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
// 	}

// 	return th.NewNATSMessageScheduler(js, logger), nil
// }

func initS3Client() (*s3.S3, error) {
	s3Config := &aws.Config{
		Credentials:      credentials.NewStaticCredentials(os.Getenv("SPACES_KEY"), os.Getenv("SPACES_SECRET"), ""),
		Endpoint:         aws.String(os.Getenv("SPACES_ENDPOINT")),
		Region:           aws.String("us-east-1"), // DigitalOcean Spaces uses "us-east-1" as default region
		S3ForcePathStyle: aws.Bool(false),         // Configures to use subdomain/virtual hosting style
	}
	newSession, err := session.NewSession(s3Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create new S3 session: %v", err)
	}
	s3Client := s3.New(newSession)
	return s3Client, nil
}

func checkEnv(logger *slog.Logger, envFile string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	logger.Info("checkEnv: working directory: " + internal.Yellow(wd))

	// Use the specified env file or default to .env
	if envFile == "" {
		envFile = ".env"
	}

	logger.Info("Loading environment file", "file", envFile)
	if err = godotenv.Overload(envFile); err != nil {
		fmt.Printf("checkEnv: did not load `%s` file: %s\n", envFile, err.Error())
	}

	requiredKeys := []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_BUCKET_ENDPOINT",
		"BASE_URL",
		"CLOUDFLARE_KV_API_TOKEN", "CLOUDFLARE_KV_NAMESPACE_ID", "CLOUDFLARE_ACCOUNT_ID",

		"FIREBASE_PROJECT_ID", "FIREBASE_CREDENTIALS",

		"LINKEDIN_AUTH_CLIENT_ID", "LINKEDIN_AUTH_CLIENT_SECRET",

		"OPENAI_API_KEY", "GEMINI_API_KEY", "DEEPSEEK_API_KEY",

		"PUSHER_APP_ID", "PUSHER_KEY", "PUSHER_SECRET", "PUSHER_CLUSTER",

		"PINECONE_DENSE_INDEX_HOST", "PINECONE_SPARSE_INDEX_HOST", "PINECONE_API_KEY",

		"SCRAPINGDOG_API_KEY",
		"SESSION_SECRET",

		/*
					SMTP_HOST="smtp.gmail.com"
			SMTP_PORT="587"
			SMTP_PASSWORD="dyvv xuge bbvp cisb"
			SMTP_USERNAME="madcow@gmail.com"
			# Example Name <no-reply@example.org>
			SMTP_FROM="Shirely at Talent Rodeo <shirely@talent.rodeo>"
		*/

		"SMTP_USERNAME", "SMTP_PASSWORD",

		"TWILIO_ACCOUNT_SID", "TWILIO_AUTH_TOKEN", "TWILIO_FROM_NUMBER",

		// consider:
		// OPENAI_DEFAULT_MODEL, LLM_DEFAULT_MODEL,

		"VAPI_API_KEY", "VAPI_ASSISTANT_ID", "VAPI_OUTBOUND_PHONE_ID",
		"VAPI_PUBLIC_API_KEY",
		"VAPI_WEBHOOK_URL",     // URL that will receive webhooks from VAPI
		"VAPI_START_CALL_PATH", // URL that we will use to start a call (we will POST to this URL to start a call)
	}

	logger.Debug(internal.Blue("checkEnv: checking environment variables"), "VAPI_WEBHOOK_URL", os.Getenv("VAPI_WEBHOOK_URL"))

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

	decoded, err := base64.StdEncoding.DecodeString(firebaseCredentials)
	if err != nil {
		return fmt.Errorf("failed to decode FIREBASE_CREDENTIALS from base64: %w", err)
	}

	var creds map[string]any
	if err := json.Unmarshal(decoded, &creds); err != nil {
		return fmt.Errorf("failed to unmarshal FIREBASE_CREDENTIALS: %w", err)
	}

	logger.Info("All required environment variables are present")
	return nil
}

// setupSessionManager initializes and returns a configured session manager.
func setupSessionManager(cfg config, FirestoreClient *internal.FirestoreClient, _ *slog.Logger) (*scs.SessionManager, error) {
	keyPairs := [][]byte{[]byte(cfg.session.secretKey), nil}
	if cfg.session.oldSecretKey != "" {
		keyPairs = append(keyPairs, []byte(cfg.session.oldSecretKey), nil)
	}

	if len(cfg.session.secretKey) < 32 {
		return nil, fmt.Errorf("SESSION_SECRET must be at least 32 bytes long")
	}

	sessionManager := scs.New()
	sessionManager.Store = scsfs.New(FirestoreClient.Client)
	sessionManager.Lifetime = 24 * time.Hour
	sessionManager.Cookie.Secure = true
	sessionManager.Cookie.HttpOnly = true
	sessionManager.Cookie.SameSite = http.SameSiteLaxMode
	sessionManager.Cookie.Name = "session"
	sessionManager.Cookie.Persist = true

	return sessionManager, nil
}

func (app *application) startServices(ctx context.Context, smtpCfg *smtp.Config) error {

	app.feedbackProcessor = feedback.NewProcessor(app.fs, app.logger)
	if err := app.feedbackProcessor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start feedback processor: %w", err)
	}

	app.subscriptionProcessor = subscription.NewProcessor(app.fs, app.pusher, app.logger)

	app.jobDispatch = internal.NewJobEventDispatch()
	app.searchResultProcessor = internal.NewSearchResultProcessor(app.fs, app.logger, app.pusher, app.jobDispatch)
	app.jobDispatch.Register("search-result-processor", app.searchResultProcessor)
	app.searchResultProcessor.Start()

	app.searchProvider = scrapingdog.NewProvider(os.Getenv("SCRAPINGDOG_API_KEY"), app.logger, scrapingdog.WithJobEventHandler(app.jobDispatch))
	app.searchScheduler = search.NewScheduler(app.fs, app.logger, app.searchProvider, app.mailer, app.config.notifications.email)

	if err := app.subscriptionProcessor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start subscription processor: %w", err)
	}
	if err := app.searchProvider.Start(ctx); err != nil {
		return fmt.Errorf("failed to start search provider: %w", err)
	}
	if err := app.searchScheduler.Start(ctx); err != nil {
		return fmt.Errorf("failed to start search scheduler: %w", err)
	}

	app.nudgeProcessor = nudge.NewProcessorWithDefaults(app.fs, smtpCfg, app.logger)
	if err := app.nudgeProcessor.Start(ctx); err != nil {
		return fmt.Errorf("failed to start nudge processor: %w", err)
	}

	return nil
}
