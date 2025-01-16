package rodeo

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/internal/utils"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/gemini"
)

func GetResumeKeywords(resume string) ([]string, error) {
	prompt, err := utils.CreateTemplate("prompts/resume-keywords.txt")
	if err != nil {
		return nil, fmt.Errorf("failed to create template: %v", err)
	}

	type output struct {
		Keywords []string `json:"keywords"`
	}

	schema, err := minds.NewResponseSchema("linkedin_search_terms", "Extracted LinkedIn search terms from resume", output{})
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %v", err)
	}

	ctx := context.Background()
	llm, err := gemini.NewProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %v", err)
	}

	req := minds.NewRequest(
		minds.Messages{{Role: minds.RoleUser, Content: prompt.Body() + "\n\n" + resume}},
		minds.WithResponseSchema(*schema),
	)

	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %v", err)
	}

	var out output
	if err := json.Unmarshal([]byte(resp.String()), &out); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return out.Keywords, nil
}
