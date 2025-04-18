package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	openai "github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

const (
	GPT41Nano = "gpt-4.1-nano" // $0.10 input	$0.025 cached output	$0.40 output
	GPT41Mini = "gpt-4.1-mini" // $0.40 input	$0.10 cached output		$1.60 output
	GPT41     = "gpt-4.1"      // $2.00 input	$0.50 cached output		$8.00 output

	GPT4oMini = "gpt-4o-mini" // $0.15 input	$0.075 cached output	$0.60 output

	O4Mini = "o4-mini" // $1.10 input	$0.275 cached output	$4.40 output
	O3Mini = "o3-mini" // $1.10 input	$0.55 cached output		$4.40 output

	maxResponseTokens = 1000
)

func GeneratePrompt(promptFile string, data any) (string, error) {
	tmpl, err := utils.CreateTemplate(promptFile)
	if err != nil {
		return "", fmt.Errorf("failed to create template %q: %w", promptFile, err)
	}

	return tmpl.Execute(data)
}

func Ask(ctx context.Context, prompt string) (string, error) {
	return AskModel(ctx, prompt, GPT41Nano)
}

func AskModel(ctx context.Context, prompt string, model string) (string, error) {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return "", fmt.Errorf("OPENAI_API_KEY is not set")
	}

	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     model,
		Messages:  []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: prompt}},
		MaxTokens: maxResponseTokens,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get response from OpenAI API: %w", err)
	}
	return resp.Choices[0].Message.Content, nil
}

func StructuredAsk[T any](ctx context.Context, name, prompt string) (T, error) {
	return StructuredAskModel[T](ctx, name, prompt, GPT41Nano)
}

func StructuredAskModel[T any](ctx context.Context, name, prompt, model string) (T, error) {
	var zero T // Zero value to return in error cases

	if os.Getenv("OPENAI_API_KEY") == "" {
		return zero, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	schema, err := GenerateJSONSchema(zero)
	if err != nil {
		return zero, fmt.Errorf("failed to generate schema: %w", err)
	}

	client := openai.NewClient(os.Getenv("OPENAI_API_KEY"))
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:     model,
		Messages:  []openai.ChatCompletionMessage{{Role: openai.ChatMessageRoleUser, Content: prompt}},
		MaxTokens: maxResponseTokens,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   name,
				Schema: schema,
				Strict: true,
			},
		},
	})
	if err != nil {
		return zero, fmt.Errorf("failed to get response from OpenAI API: %w", err)
	}

	var result T
	if err := json.Unmarshal([]byte(resp.Choices[0].Message.Content), &result); err != nil {
		return zero, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return result, nil
}

func GenerateJSONSchema(v any) (*jsonschema.Definition, error) {
	schema, err := minds.GenerateSchema(v)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %w", err)
	}

	return ConvertSchema(schema)
}

func ConvertSchema(schema *minds.Definition) (*jsonschema.Definition, error) {
	// a minds.Definition and jsonschema.Definition are exactly the same.  just deep copy everything
	// and return it
	//
	// type Definition struct {
	// 	// Type specifies the data type of the schema.
	// 	Type DataType `json:"type,omitempty"`
	// 	// Description is the description of the schema.
	// 	Description string `json:"description,omitempty"`
	// 	// Enum is used to restrict a value to a fixed set of values. It must be an array with at least
	// 	// one element, where each element is unique. You will probably only use this with strings.
	// 	Enum []string `json:"enum,omitempty"`
	// 	// Properties describes the properties of an object, if the schema type is Object.
	// 	Properties map[string]Definition `json:"properties,omitempty"`
	// 	// Required specifies which properties are required, if the schema type is Object.
	// 	Required []string `json:"required,omitempty"`
	// 	// Items specifies which data type an array contains, if the schema type is Array.
	// 	Items *Definition `json:"items,omitempty"`
	// 	// AdditionalProperties is used to control the handling of properties in an object
	// 	// that are not explicitly defined in the properties section of the schema. example:
	// 	// additionalProperties: true
	// 	// additionalProperties: false
	// 	// additionalProperties: Definition{Type: String}
	// 	AdditionalProperties any `json:"additionalProperties,omitempty"`
	// }

	result := jsonschema.Definition{
		Type:                 jsonschema.DataType(schema.Type),
		Description:          schema.Description,
		Enum:                 schema.Enum,
		Required:             schema.Required,
		AdditionalProperties: schema.AdditionalProperties,
	}

	// Deep copy properties map if it exists
	if schema.Properties != nil {
		result.Properties = make(map[string]jsonschema.Definition)
		for key, prop := range schema.Properties {
			propCopy, err := ConvertSchema(&prop)
			if err != nil {
				return nil, fmt.Errorf("failed to convert property %s: %w", key, err)
			}
			result.Properties[key] = *propCopy
		}
	}

	// Deep copy Items if it exists
	if schema.Items != nil {
		itemsCopy, err := ConvertSchema(schema.Items)
		if err != nil {
			return nil, fmt.Errorf("failed to convert items: %w", err)
		}
		result.Items = itemsCopy
	}

	return &result, nil
}
