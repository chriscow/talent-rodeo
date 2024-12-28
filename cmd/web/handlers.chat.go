package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"talent-rodeo/cmd/web/templates/pages"

	"github.com/justinas/nosurf"
	"github.com/sashabaranov/go-openai"
)

type Message struct {
	Role    string
	Content string
}

func (app *application) handleChat(w http.ResponseWriter, r *http.Request) {
	csrfToken := nosurf.Token(r)

	if r.Method == "GET" {
		user := contextGetAuthenticatedUser(r)
		component := pages.ChatIndex(csrfToken, user)
		component.Render(r.Context(), w)
		return
	}

	if r.Method != "POST" {
		app.badRequest(w, r, fmt.Errorf("method not allowed"))
		return
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		app.serverError(w, r, fmt.Errorf("OpenAI API key is not set"))
		return
	}

	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))

	systemPrompt := r.FormValue("system_prompt")
	userMessage := r.FormValue("message")

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: userMessage,
		},
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model:    openai.GPT4,
			Messages: messages,
		},
	)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Render the messages component with the new messages
	messages = append(messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleAssistant,
		Content: resp.Choices[0].Message.Content,
	})

	messagesComponent := pages.MessagesList(messages)
	messagesComponent.Render(context.Background(), w)
}

func (app *application) handleMessages(w http.ResponseWriter, r *http.Request) {
	component := pages.MessagesList([]openai.ChatCompletionMessage{})
	component.Render(context.Background(), w)
}
