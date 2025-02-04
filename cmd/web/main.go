package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"talent-rodeo/internal/auth"
	"talent-rodeo/internal/database"
	"talent-rodeo/internal/smtp"
	"talent-rodeo/internal/version"

	"github.com/delaneyj/toolbelt"
	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	natsserver "github.com/nats-io/nats-server/v2/server"
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
	baseURL   string
	httpPort  int
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
	nats          *embeddednats.Server
	pusher        *pusher.Client
	keywordsMutex sync.RWMutex
	natsShutdown  chan struct{}
}

func run(logger *slog.Logger) error {
	if err := checkEnv(); err != nil {
		return err
	}

	var cfg config

	flag.StringVar(&cfg.baseURL, "base-url", os.Getenv("BASE_URL"), "base URL for the application")
	flag.IntVar(&cfg.httpPort, "http-port", 2304, "port to listen on for HTTP requests")
	flag.StringVar(&cfg.autoHTTPS.domain, "auto-https-domain", "", "domain to enable automatic HTTPS for")
	flag.StringVar(&cfg.autoHTTPS.email, "auto-https-email", "admin@example.com", "contact email address for problems with LetsEncrypt certificates")
	flag.BoolVar(&cfg.autoHTTPS.staging, "auto-https-staging", false, "use LetsEncrypt staging environment")
	flag.StringVar(&cfg.cookie.secretKey, "cookie-secret-key", os.Getenv("SESSION_SECRET"), "secret key for cookie authentication/encryption")
	flag.StringVar(&cfg.db.dsn, "db-dsn", "db.sqlite", "sqlite3 DSN")
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

	// Use environment DSN if available, fallback to flag value
	if envDSN := os.Getenv("DB_DSN"); envDSN != "" {
		cfg.db.dsn = envDSN
		logger.Debug("using database DSN from environment",
			"dsn_type", func() string {
				if strings.HasPrefix(envDSN, "postgres") {
					return "PostgreSQL"
				}
				return "SQLite"
			}(),
			"dsn_prefix", envDSN[:10]+"...")
	}

	var db *database.DB
	if strings.HasPrefix(cfg.db.dsn, "postgres") {
		logger.Info("initializing PostgreSQL database")
		pgDB, err := database.NewPG(cfg.db.dsn, cfg.db.automigrate)
		if err != nil {
			logger.Error("failed to connect to PostgreSQL",
				"error", err,
				"dsn_prefix", cfg.db.dsn[:10]+"...")
			return fmt.Errorf("failed to connect to PostgreSQL: %w", err)
		}
		logger.Debug("creating database instance from PostgreSQL connection")
		db = database.NewFromPG(pgDB, logger)
		logger.Info("PostgreSQL database initialized successfully")
	} else {
		logger.Info("initializing SQLite database", "path", cfg.db.dsn)
		var err error
		opts := []database.Option{
			database.WithLogger(logger),
			database.WithForeignKeys(),
		}
		if cfg.db.automigrate {
			logger.Debug("enabling automatic migrations for SQLite")
			opts = append(opts, database.WithTasksTable())
		}
		db, err = database.New(cfg.db.dsn, opts...)
		if err != nil {
			logger.Error("failed to connect to SQLite",
				"error", err,
				"path", cfg.db.dsn)
			return fmt.Errorf("failed to connect to SQLite: %w", err)
		}
		logger.Info("SQLite database initialized successfully")
	}
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

	var ns *embeddednats.Server
	ctx, cancel := context.WithCancel(context.Background())

	// Create a channel to signal when NATS is shut down
	natsShutdown := make(chan struct{})
	defer func() {
		if ns != nil {
			logger.Info("shutting down NATS server")
			// Close NATS first
			ns.Close()
			close(natsShutdown)
			logger.Info("NATS server shutdown complete")
		}
		cancel()
	}()

	ns, err = func() (*embeddednats.Server, error) {
		natsPort, err := toolbelt.FreePort()
		if err != nil {
			return nil, fmt.Errorf("error getting free port: %w", err)
		}

		ns, err := embeddednats.New(ctx, embeddednats.WithNATSServerOptions(&natsserver.Options{
			JetStream: true,
			Port:      natsPort,
		}))

		if err != nil {
			return nil, fmt.Errorf("error creating embedded nats server: %w", err)
		}

		ns.WaitForServer()
		return ns, nil
	}()

	if err != nil {
		return fmt.Errorf("error setting up NATS: %w", err)
	}

	logger.Debug("pusher config", "app_id", os.Getenv("PUSHER_APP_ID"), "key",
		os.Getenv("PUSHER_KEY"), "secret", os.Getenv("PUSHER_SECRET"), "cluster", os.Getenv("PUSHER_CLUSTER"))

	pusherClient := pusher.Client{
		AppID:   os.Getenv("PUSHER_APP_ID"),
		Key:     os.Getenv("PUSHER_KEY"),
		Secret:  os.Getenv("PUSHER_SECRET"),
		Cluster: os.Getenv("PUSHER_CLUSTER"),
		Secure:  true,
	}

	app := &application{
		config:       cfg,
		db:           db,
		logger:       logger,
		mailer:       mailer,
		sessionStore: sessionStore,
		nats:         ns,
		pusher:       &pusherClient,
		natsShutdown: natsShutdown,
	}

	if cfg.autoHTTPS.domain != "" {
		return app.serveAutoHTTPS()
	}

	return app.serveHTTP()
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
