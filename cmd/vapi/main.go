package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/assets"

	"github.com/joho/godotenv"
)

type config struct {
	port           int
	webhookURL     string
	environment    string
	command        string
	customerNumber string
	customerName   string
	resume         string
}

type application struct {
	config config
	logger *slog.Logger
}

func main() {
	var cfg config

	// Server flags
	flag.IntVar(&cfg.port, "port", 4000, "API server port")
	flag.StringVar(&cfg.webhookURL, "webhook-url", "", "VAPI webhook URL")
	flag.StringVar(&cfg.environment, "env", "development", "Environment (development|staging|production)")

	// Command flags
	flag.StringVar(&cfg.command, "cmd", "server", "Command to run (server|call)")
	flag.StringVar(&cfg.customerNumber, "number", "+14258295443", "Phone number to call (required for call command)")
	flag.StringVar(&cfg.customerName, "name", "Chris Cowherd", "Customer name (required for call command)")
	flag.StringVar(&cfg.resume, "resume", "test-data/chris-cowherd.resume.yaml", "Resume to use for call command")
	flag.Parse()

	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		fmt.Printf("Warning: .env file not found: %v\n", err)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	app := &application{
		config: cfg,
		logger: logger,
	}

	// Handle different commands
	switch cfg.command {
	case "server":
		app.runServer()
	case "call":
		// Override webhook URL from environment if not set via flag
		if cfg.webhookURL == "" {
			cfg.webhookURL = os.Getenv("VAPI_WEBHOOK_URL")
			if cfg.webhookURL == "" {
				fmt.Println("Error: webhook URL must be provided via -webhook-url flag or VAPI_WEBHOOK_URL environment variable")
				os.Exit(1)
			}
		}

		if os.Getenv("VAPI_API_KEY") == "" {
			fmt.Println("Error: VAPI_API_KEY environment variable must be set")
			os.Exit(1)
		}

		if cfg.customerNumber == "" {
			fmt.Println("Error: phone number must be provided via -number flag for call command")
			os.Exit(1)
		}

		resume, err := assets.EmbeddedFiles.ReadFile(cfg.resume)
		if err != nil {
			logger.Error("failed to read resume", "error", err)
			os.Exit(1)
		}

		if err := app.createCall(context.Background(), cfg.customerNumber, cfg.customerName, string(resume)); err != nil {
			logger.Error("failed to create call", "error", err)
			os.Exit(1)
		}
	default:
		fmt.Printf("Error: unknown command %q\n", cfg.command)
		os.Exit(1)
	}
}
