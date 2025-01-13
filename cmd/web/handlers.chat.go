package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"talent-rodeo/internal/response"

	"github.com/sashabaranov/go-openai"
)

type Message struct {
	Role    string
	Content string
}

func (app *application) handleChat(w http.ResponseWriter, r *http.Request) {

	if r.Method == "GET" {
		data := app.newTemplateData(r)
		if err := response.Page(w, http.StatusOK, data, "pages/chat_index.tmpl"); err != nil {
			app.serverError(w, r, err)
		}
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

	// messagesComponent := pages.MessagesList(messages)
	// messagesComponent.Render(context.Background(), w)
	data := app.newTemplateData(r)

	if err := response.Page(w, http.StatusOK, data, "pages/chat_index.tmpl"); err != nil {
		app.serverError(w, r, err)
	}

}

func (app *application) handleMessages(w http.ResponseWriter, r *http.Request) {
	if err := response.Partial(w, http.StatusOK, nil, "partials/messages_list.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}
