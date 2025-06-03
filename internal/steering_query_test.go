package internal

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/chriscow/minds/tools"
	"github.com/chriscow/vapi-go"
)

func TestSteeringQueryFromTranscript(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in CI")
	}

	// Read the test data file
	data, err := os.ReadFile("../testdata/vapi/create-call-response.json")
	if err != nil {
		t.Fatalf("Failed to read test data: %v", err)
	}

	// Parse the JSON into a struct that has the messages field
	var callResult vapi.Call
	if err := json.Unmarshal(data, &callResult); err != nil {
		t.Fatalf("Failed to unmarshal test data: %v", err)
	}

	// Convert the messages to a transcript
	transcript := NewTranscriptFromCallResult(callResult)

	// Verify the conversion
	if len(transcript.Messages) == 0 {
		t.Error("Expected non-empty transcript messages")
	}

	// Create a steering query from the transcript
	sq, err := GenerateSteeringQuery(context.Background(), transcript, tools.WithModel(tools.GPT41Nano))
	if err != nil {
		t.Fatalf("Failed to create steering query: %v", err)
	}

	if err := sq.Validate(); err != nil {
		t.Errorf("Steering query validation failed: %v", err)
	}
}

func TestCleanJSONFromMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "JSON with json markdown block",
			input:    "```json\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "JSON with generic markdown block",
			input:    "```\n{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "JSON without markdown blocks",
			input:    "{\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "JSON with extra whitespace",
			input:    "  \n  ```json\n  {\"key\": \"value\"}  \n  ```  \n  ",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "JSON with only opening markdown block",
			input:    "```json\n{\"key\": \"value\"}",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "JSON with only closing markdown block",
			input:    "{\"key\": \"value\"}\n```",
			expected: "{\"key\": \"value\"}",
		},
		{
			name:     "Empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "Only whitespace",
			input:    "   \n  \t  \n   ",
			expected: "",
		},
		{
			name:     "Only markdown blocks",
			input:    "```json\n```",
			expected: "",
		},
		{
			name:     "Complex JSON with nested objects",
			input:    "```json\n{\n  \"user\": {\n    \"name\": \"John\",\n    \"age\": 30\n  },\n  \"active\": true\n}\n```",
			expected: "{\n  \"user\": {\n    \"name\": \"John\",\n    \"age\": 30\n  },\n  \"active\": true\n}",
		},
		{
			name:     "JSON with newlines and formatting",
			input:    "```json\n[\n  \"item1\",\n  \"item2\",\n  \"item3\"\n]\n```",
			expected: "[\n  \"item1\",\n  \"item2\",\n  \"item3\"\n]",
		},
		{
			name:     "JSON with text before and after markdown",
			input:    "Here is the JSON:\n```json\n{\"result\": true}\n```\nThat's it!",
			expected: "Here is the JSON:\n```json\n{\"result\": true}\n```\nThat's it!",
		},
		{
			name:     "Multiple code blocks (only removes outer ones)",
			input:    "```json\n{\"code\": \"```inner```\"}\n```",
			expected: "{\"code\": \"```inner```\"}",
		},
		{
			name:     "JSON with unicode characters",
			input:    "```json\n{\"message\": \"Hello 🌍\", \"emoji\": \"😀\"}\n```",
			expected: "{\"message\": \"Hello 🌍\", \"emoji\": \"😀\"}",
		},
		{
			name:     "Malformed markdown with extra backticks",
			input:    "````json\n{\"key\": \"value\"}\n````",
			expected: "`json\n{\"key\": \"value\"}\n`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanJSONFromMarkdown(tt.input)
			if result != tt.expected {
				t.Errorf("cleanJSONFromMarkdown() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestExtractSteeringQuery(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	ctx := context.Background()

	// Define the complex LLM response string
	complexLLMResponse := `Based on the transcript, here's the analysis and the resulting steering query:

1. **Query**: "Go (Golang)"
   - The user explicitly states: "I really would like to focus on developing in Go back end services ideally, that work with LLMs."
   - Confirms the focus on Go when asked: "Yeah. That's right. No. That's right. That's right."

2. **MoveToward**: "Backend development with large language models (LLMs) and generative AI, preferably in smaller companies or startups, with fully remote work options."
   - User mentions: "that work with LLMs"
   - Expresses preference for smaller companies: "I would lean more towards, uh, a smaller company than, you know, something that's like a start up or something like that."
   - States remote preference: "I think I'd just like to be fully remote."

3. **MoveAway**: "Frontend development and frontend frameworks."
   - User clearly states: "I'm not really a strong front end developer, so I probably need to avoid uh, you know, front end development or front end frameworks."

4. **RerankingQuery**: "Backend development using Go (Golang) with large language models (LLMs) and generative AI, preferably in smaller companies or startups with fully remote work options, while avoiding frontend development and frontend frameworks."

Final JSON output:

` + "```json\n" + `{
  "Query": "Go (Golang)",
  "MoveToward": "Backend development with large language models (LLMs) and generative AI, preferably in smaller companies or startups, with fully remote work options",
  "MoveAway": "Frontend development and frontend frameworks",
  "RerankingQuery": "Backend development using Go (Golang) with large language models (LLMs) and generative AI, preferably in smaller companies or startups with fully remote work options, while avoiding frontend development and frontend frameworks"
}` + "```"

	tests := []struct {
		name     string
		input    string
		expected *SteeringQuery
		wantErr  bool
	}{
		{
			name:  "Complete LLM response with analysis and JSON",
			input: complexLLMResponse,
			expected: &SteeringQuery{
				Query:       "Go (Golang)",
				MoveToward:  "Backend development with large language models (LLMs) and generative AI, preferably in smaller companies or startups, with fully remote work options",
				MoveAway:    "Frontend development and frontend frameworks",
				ForceToward: 0.8, // Set by extractSteeringQuery
				ForceAway:   0.5, // Set by extractSteeringQuery
				RerankQuery: "Backend development using Go (Golang) with large language models (LLMs) and generative AI, preferably in smaller companies or startups with fully remote work options, while avoiding frontend development and frontend frameworks",
			},
			wantErr: false,
		},
		{
			name: "JSON with answer tags",
			input: "<answer>\n" +
				`{
  "Query": "Python",
  "MoveToward": "Machine learning and data science",
  "MoveAway": null,
  "RerankQuery": "Python development focused on machine learning and data science"
}` + "\n</answer>",
			expected: &SteeringQuery{
				Query:       "Python",
				MoveToward:  "Machine learning and data science",
				MoveAway:    "",
				ForceToward: 0.8,
				ForceAway:   0.5,
				RerankQuery: "Python development focused on machine learning and data science",
			},
			wantErr: false,
		},
		{
			name: "JSON in markdown code block",
			input: "Here is the steering query:\n\n" + "```json\n" +
				`{
  "Query": "JavaScript",
  "MoveToward": null,
  "MoveAway": "Legacy systems",
  "RerankQuery": "Modern JavaScript development avoiding legacy systems"
}` + "\n```",
			expected: &SteeringQuery{
				Query:       "JavaScript",
				MoveToward:  "",
				MoveAway:    "Legacy systems",
				ForceToward: 0.8,
				ForceAway:   0.5,
				RerankQuery: "Modern JavaScript development avoiding legacy systems",
			},
			wantErr: false,
		},
		{
			name: "Plain JSON without wrapping",
			input: `{
  "Query": "Rust",
  "MoveToward": "Systems programming",
  "MoveAway": "Web development",
  "RerankQuery": "Rust systems programming avoiding web development"
}`,
			expected: &SteeringQuery{
				Query:       "Rust",
				MoveToward:  "Systems programming",
				MoveAway:    "Web development",
				ForceToward: 0.8,
				ForceAway:   0.5,
				RerankQuery: "Rust systems programming avoiding web development",
			},
			wantErr: false,
		},
		{
			name:    "Invalid JSON",
			input:   `{"Query": "Test", "invalid": json}`,
			wantErr: true,
		},
		{
			name:    "Missing required fields",
			input:   `{"Query": "Test"}`,
			wantErr: true,
		},
		{
			name:    "Empty input",
			input:   "",
			wantErr: true,
		},
		{
			name: "Answer tags with invalid JSON",
			input: "<answer>\n" +
				`{
  "Query": "Test",
  "invalid": json
}` + "\n</answer>",
			wantErr: true,
		},
		{
			name: "Malformed answer tags",
			input: "<answer>\n" +
				`{"Query": "Test", "RerankQuery": "Test query"}` + "\n</answer\nMissing closing tag",
			wantErr: true,
		},
		{
			name: "Answer tags in wrong order",
			input: "</answer>\n" +
				`{"Query": "Test", "RerankQuery": "Test query"}` + "\n<answer>",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := extractSteeringQuery(ctx, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("extractSteeringQuery() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("extractSteeringQuery() unexpected error: %v", err)
				return
			}

			if result == nil {
				t.Fatal("extractSteeringQuery() returned nil result")
			}

			// Compare all fields
			if result.Query != tt.expected.Query {
				t.Errorf("Query mismatch: got %q, expected %q", result.Query, tt.expected.Query)
			}

			if result.MoveToward != tt.expected.MoveToward {
				t.Errorf("MoveToward mismatch: got %v, expected %v", result.MoveToward, tt.expected.MoveToward)
			}

			if result.MoveAway != tt.expected.MoveAway {
				t.Errorf("MoveAway mismatch: got %v, expected %v", result.MoveAway, tt.expected.MoveAway)
			}

			if result.RerankQuery != tt.expected.RerankQuery {
				t.Errorf("RerankQuery mismatch: got %q, expected %q", result.RerankQuery, tt.expected.RerankQuery)
			}

			if result.ForceToward != tt.expected.ForceToward {
				t.Errorf("ForceToward mismatch: got %f, expected %f", result.ForceToward, tt.expected.ForceToward)
			}

			if result.ForceAway != tt.expected.ForceAway {
				t.Errorf("ForceAway mismatch: got %f, expected %f", result.ForceAway, tt.expected.ForceAway)
			}
		})
	}
}

func qual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
