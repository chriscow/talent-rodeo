// Package main provides a voice chat application that enables real-time
// communication between users via Twilio phone calls and OpenAI's streaming API.
package main

import (
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"phone-llm/internal/voice"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/lmittmann/tint"
)

const (
	GPT4oRealtimePreview         = "gpt-4o-realtime-preview"
	GPT4oRealtimePreview20241001 = "gpt-4o-realtime-preview-2024-10-01"
)

var (
	gray   = color.New(color.FgHiBlack).SprintFunc()
	red    = color.New(color.FgRed).SprintFunc()
	yellow = color.New(color.FgYellow).SprintFunc()

	green   = color.New(color.FgGreen).SprintFunc()
	greenBk = color.New(color.BgGreen).SprintFunc()

	blue    = color.New(color.FgBlue).SprintFunc()
	magenta = color.New(color.FgMagenta).SprintFunc()
	cyan    = color.New(color.FgCyan).SprintFunc()
	white   = color.New(color.FgWhite).SprintFunc()
)

// Config holds application configuration settings
type Config struct {
	AppDomain        string
	OpenAIAPIKey     string
	Port             string
	AudioBufferDelay time.Duration
	LogFile          string
}

// initLogger sets up structured logging
func initLogger(logPath string) *slog.Logger {
	return slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug, AddSource: true}))
}

// setupRoutes configures HTTP routes
func setupRoutes(config *Config, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	mux.HandleFunc("/voice", func(w http.ResponseWriter, r *http.Request) {
		callID := r.FormValue("CallSid")
		callerNumber := r.FormValue("From")
		calledNumber := r.FormValue("To")
		direction := r.FormValue("Direction")

		logger.Info(green("Incoming call"), "number", callerNumber, "direction", direction, "call_id", callID, "to", calledNumber)

		// Generate TwiML response for incoming calls
		twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
		<Response>
			<Connect>
				<Stream url="wss://%s/stream"/>
			</Connect>
			<Start>
				<Transcription statusCallbackUrl="https://%s/transcription"/>
			</Start>
		</Response>`, config.AppDomain, config.AppDomain)

		logger.Info(fmt.Sprintf("[%s] sending TwiML response: %s\n", gray("twilio"), twiml))

		w.Header().Set("Content-Type", "text/xml")
		w.Write([]byte(twiml))
	})

	mux.HandleFunc("/transcription", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("["+gray("twilio")+"] "+green("Transcription callback"), "from", r.RemoteAddr)
		if err := r.ParseForm(); err != nil {
			logger.Error("Failed to parse form data", "error", err)
			return
		}

		for key, value := range r.Form {
			logger.Info("["+gray("twilio")+"] "+green("Form data"), key, strings.Join(value, ","))
		}
	})

	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		logger.Info("["+gray("twilio")+"] ", greenBk("ws connection from: "), r.RemoteAddr)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Error("WebSocket upgrade failed", "error", err)
			return
		}

		session, err := voice.NewSession(conn, logger)
		if err != nil {
			logger.Error("Failed to create chat session", "error", err)
			conn.Close()
			return
		}

		if err := session.Start(r.Context()); err != nil {
			logger.Error("Chat session failed", "error", err)
		}
	})

	return mux
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	// Parse command line flags
	var config Config
	flag.DurationVar(&config.AudioBufferDelay, "buffer", 5*time.Second, "Audio buffer delay duration")
	flag.StringVar(&config.LogFile, "log", "app.log", "Log file path")
	flag.StringVar(&config.Port, "http-port", "2304", "HTTP server port")
	flag.StringVar(&config.AppDomain, "app-domain", os.Getenv("APP_DOMAIN"), "Application domain")
	flag.StringVar(&config.OpenAIAPIKey, "openai-api-key", os.Getenv("OPENAI_API_KEY"), "OpenAI API key")
	flag.Parse()

	if config.OpenAIAPIKey == "" {
		log.Fatal("OpenAI API key is required or OPENAI_API_KEY environment variable set")
	}

	if config.AppDomain == "" {
		log.Fatal("App domain is required or APP_DOMAIN environment variable set")
	}

	logger := initLogger(config.LogFile)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", config.Port),
		Handler: setupRoutes(&config, logger),
	}

	logger.Info("Server starting", "port", config.Port)
	fmt.Println("Server starting on port", config.Port)
	if err := server.ListenAndServe(); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
