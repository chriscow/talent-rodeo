package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"runtime/debug"
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

func main2() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
		return
	}

	// Check for OpenAI API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	// Initialize handlers
	h, err := NewHandler()
	if err != nil {
		log.Fatal(err)
	}

	// Setup routes
	http.HandleFunc("/", h.InterviewPage)
	http.HandleFunc("/api/interview/audio", h.HandleAudio)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "2304"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal(err)
	}
}

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
}

type application struct {
	config       config
	db           *database.DB
	logger       *slog.Logger
	mailer       *smtp.Mailer
	sessionStore *sessions.CookieStore
	wg           sync.WaitGroup
	nats         *embeddednats.Server
	pusher       *pusher.Client
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

	db, err := database.New(cfg.db.dsn, cfg.db.automigrate)
	if err != nil {
		return err
	}
	defer db.Close()

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

	ns, err := setupNATS(ctx)
	if err != nil {
		return fmt.Errorf("error setting up NATS: %w", err)
	}

	pusherClient := pusher.Client{
		AppID:   "1921779",
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

func setupNATS(ctx context.Context) (*embeddednats.Server, error) {
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
}

func checkEnv() error {
	err := godotenv.Load()
	if err != nil {
		fmt.Printf("Error loading .env file: %s\n", err.Error())
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		return fmt.Errorf("BASE_URL environment variable is required")
	}

	sessionSecret := os.Getenv("SESSION_SECRET")
	if sessionSecret == "" {
		return fmt.Errorf("SESSION_SECRET environment variable is required")
	}

	smtpUser := os.Getenv("SMTP_USERNAME")
	if smtpUser == "" {
		return fmt.Errorf("SMTP_USERNAME environment variable is required")
	}

	smtpPwd := os.Getenv("SMTP_PASSWORD")
	if smtpPwd == "" {
		return fmt.Errorf("SMTP_PASSWORD environment variable is required")
	}

	if os.Getenv("LINKEDIN_AUTH_CLIENT_ID") == "" {
		return fmt.Errorf("LINKEDIN_AUTH_CLIENT_ID environment variable is required")
	}

	if os.Getenv("LINKEDIN_AUTH_CLIENT_SECRET") == "" {
		return fmt.Errorf("LINKEDIN_AUTH_CLIENT_SECRET environment variable is required")
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is required")
	}

	return nil
}
