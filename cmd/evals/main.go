package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"talent-rodeo/cmd/evals/conversations"
	"talent-rodeo/cmd/evals/server"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file")
	}

	var rootCmd = &cobra.Command{Use: "recruit-eval"}

	ctx, cancel := context.WithCancel(context.Background())

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigint
		cancel()
	}()

	// CLI Command: Generate Conversations
	var generateCmd = &cobra.Command{
		Use:   "generate",
		Short: "Generate LLM conversations",
		Run: func(cmd *cobra.Command, args []string) {
			conversations.RunConversations(ctx)
		},
	}
	rootCmd.AddCommand(generateCmd)

	// CLI Command: Start Web Server
	var serverCmd = &cobra.Command{
		Use:   "serve",
		Short: "Start the web server",
		Run: func(cmd *cobra.Command, args []string) {
			server.StartServer()
		},
	}
	rootCmd.AddCommand(serverCmd)

	// Execute CLI
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
}
