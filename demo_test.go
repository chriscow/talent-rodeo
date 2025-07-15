//go:build integration

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"talent-rodeo/internal"
	"talent-rodeo/internal/chat"

	"github.com/sashabaranov/go-openai"
)

func main() {
	// Check for OpenAI API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		fmt.Println("Please set OPENAI_API_KEY environment variable")
		fmt.Println("Example: export OPENAI_API_KEY=your_key_here")
		os.Exit(1)
	}

	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Starting hybrid completion framework demo")

	// Create OpenAI client
	openAIClient := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	// Create service dependencies
	serviceDeps := &internal.ServiceDeps{
		Logger: logger,
		// For testing, we can leave other dependencies as nil
	}

	// Create chat service (nil Firestore client for demo)
	chatService := chat.NewService(nil, openAIClient, serviceDeps, logger)

	// Create test framework
	framework := chat.NewCompletionTestFramework(chatService, logger)

	// Run demo with our sample conversation
	ctx := context.Background()
	
	logger.Info("Loading sample conversation...")
	testData, err := framework.LoadConversationData("sample/basic_conversation.json")
	if err != nil {
		logger.Error("Failed to load test data", "error", err)
		os.Exit(1)
	}

	logger.Info("Running consistency test", "test_name", testData.Name, "runs", 3)
	
	// Run the test 3 times to demonstrate consistency
	analysis, err := framework.RunConsistencyTest(ctx, testData, 3)
	if err != nil {
		logger.Error("Failed to run consistency test", "error", err)
		os.Exit(1)
	}

	// Display results
	fmt.Println("\n===== DEMO RESULTS =====")
	fmt.Printf("Test: %s\n", analysis.TestName)
	fmt.Printf("Total Runs: %d\n", analysis.TotalRuns)
	fmt.Printf("Successful Runs: %d\n", analysis.SuccessfulRuns)
	fmt.Printf("Success Rate: %.2f%%\n", analysis.SuccessRate*100)
	fmt.Printf("Average Execution Time: %v\n", analysis.AverageExecutionTime)
	fmt.Printf("Completion Score Variance: %.4f\n", analysis.CompletionScoreVariance)

	fmt.Println("\nTool Call Consistency:")
	for tool, count := range analysis.ToolCallConsistency {
		fmt.Printf("  %s: %d/%d runs\n", tool, count, analysis.TotalRuns)
	}

	fmt.Println("\nCompletion Type Consistency:")
	for completionType, count := range analysis.CompletionConsistency {
		fmt.Printf("  %s: %d/%d runs\n", completionType, count, analysis.TotalRuns)
	}

	fmt.Println("\n===== INDIVIDUAL RUN DETAILS =====")
	for i, result := range analysis.Results {
		fmt.Printf("Run %d:\n", i+1)
		fmt.Printf("  Success: %v\n", result.Success)
		fmt.Printf("  Execution Time: %v\n", result.ExecutionTime)
		if result.CompletionStatus != nil {
			fmt.Printf("  Completion Score: %.2f\n", result.CompletionStatus.CompletionScore)
			fmt.Printf("  Can Complete: %v\n", result.CompletionStatus.CanComplete)
			fmt.Printf("  Hard Complete: %v\n", result.CompletionStatus.HardComplete)
			fmt.Printf("  Soft Complete: %v\n", result.CompletionStatus.SoftComplete)
		}
		if result.ErrorMessage != "" {
			fmt.Printf("  Error: %s\n", result.ErrorMessage)
		}
		fmt.Println()
	}

	logger.Info("Demo completed successfully")
}