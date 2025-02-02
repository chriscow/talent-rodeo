package conversations

import (
	"fmt"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/handlers"
	"github.com/chriscow/minds/providers/openai"
)

type summarizer struct {
	handler minds.ThreadHandler
}

// ConversationSummarizer creates middleware that asynchronously summarizes conversations
func ConversationSummarizer(provider *openai.Provider, systemMsg string) minds.Middleware {
	return &summarizer{
		handler: handlers.NewSummarizer(provider, systemMsg),
	}
}

func (s *summarizer) Wrap(next minds.ThreadHandler) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, nextHandler minds.ThreadHandler) (minds.ThreadContext, error) {
		length := countNonSystemMessages(tc)
		if length < 10 {
			return next.HandleThread(tc, nextHandler)
		}

		// Create a channel to receive the summary
		summaryCh := make(chan string, 1)

		// Start goroutine to summarize conversation
		go func() {
			summary, err := s.handler.HandleThread(tc, &minds.NoopThreadHandler{})
			if err != nil {
				summaryCh <- ""
				return
			}
			summaryCh <- summary.Messages()[len(summary.Messages())-1].Content
		}()

		// Run the wrapped handler
		resultTC, err := next.HandleThread(tc, nextHandler)
		if err != nil {
			return resultTC, err
		}

		// Wait for summary
		summary := <-summaryCh

		// If summarization was successful, truncate conversation
		if summary != "" {
			messages := resultTC.Messages()
			if len(messages) == 0 {
				return resultTC, nil
			}

			// Create new messages slice with system prompt
			truncated := []minds.Message{{
				Role:    "system",
				Content: summary, // this contains the original system message plus updated summary
			}}

			// Keep last two messages if they exist
			if len(messages) > 2 {
				truncated = append(truncated, messages[len(messages)-2:]...)
			} else if len(messages) > 1 {
				truncated = append(truncated, messages[1:]...)
			}

			// Create new thread context with truncated messages
			resultTC = resultTC.WithMessages(truncated...)
			fmt.Printf("[%s] Truncated conversation to %d messages\n", yellow.Sprintf("SUMMARY"), len(truncated))
		}

		return next.HandleThread(resultTC, nextHandler)
	})
}

// countNonSystemMessages returns the number of non-system messages in the thread
func countNonSystemMessages(tc minds.ThreadContext) int {
	count := 0
	for _, msg := range tc.Messages() {
		if msg.Role != "system" {
			count++
		}
	}
	return count
}
