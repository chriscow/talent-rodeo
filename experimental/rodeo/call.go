package rodeo

import (
	"context"
	"fmt"
	"talent-rodeo/internal/vapi"

	"github.com/chriscow/minds"
)

func callCandidate(ctx context.Context, sessionID, systemPrompt string, status StatusSender) (*vapi.Call, error) {
	status.SendStatus(sessionID, "info", "Calling candidate ...")

	assistantID := "30709dea-4c26-457a-96a0-4ee7488f6b72"
	outboundPhoneID := "16bbd5bb-2e5a-4a3b-937b-0665e1648dff"

	modelConfig := vapi.DefaultModelConfig
	modelConfig.Messages = []vapi.ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	analysisPrompt := `
	You are an AI assistant tasked with analyzing a transcript of a candidate's interview call. Your goal is to extract key information about the candidate.
	
	#### Instructions:
	1. Carefully analyze the transcript of the call provided below.
	2. Extract and structure all relevant information into the fields of the JSON.
	3. If certain information is not explicitly mentioned but can be reasonably inferred, include it in the relevant field and mark it as "inferred."
	4. Ensure all fields are filled to the best of your ability based on the transcript.
	
	#### **Output: Formatted CandidateInfo**
	Provide the structured information as JSON formatted output.
		
	Focus on accuracy and completeness to ensure the structured output provides 
	valuable insights for identifying job opportunities for the candidate.`

	analysisSchema, err := minds.GenerateSchema(CandidateInfo{})
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to generate schema")
		return nil, fmt.Errorf("callCandidate: failed to generate schema: %w", err)
	}

	return vapi.CreateCall(ctx, vapi.Call{
		AssistantID:   &assistantID,
		PhoneNumberID: &outboundPhoneID,
		AssistantOverrides: &vapi.Assistant{
			Model: &modelConfig,
			AnalysisPlan: &vapi.AnalysisPlan{
				StructuredDataPrompt: &analysisPrompt,
				StructuredDataSchema: analysisSchema,
			},
		},
		Customer: &vapi.Customer{
			Number: "+14258295443",
			Name:   "Chris Cowherd",
		},
	})
}

type EndCallArgs struct {
	CallID      string `json:"call_id"`
	Disposition string `json:"disposition" enum:"completed,missed,failed,other"`
	Notes       string `json:"notes"`
}

type endCallTool struct {
	cancel context.CancelFunc
}

func (t *endCallTool) Type() string {
	return "end_call"
}

func (t *endCallTool) Name() string {
	return "end_call"
}

func (t *endCallTool) Description() string {
	return "End the current phone call and record its disposition"
}

func (t *endCallTool) Parameters() minds.Definition {
	definition, _ := minds.GenerateSchema(EndCallArgs{})
	return *definition
}

func (t *endCallTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	t.cancel()
	return nil, nil
}

func NewEndCallTool(cancel context.CancelFunc) (minds.Tool, error) {
	return &endCallTool{cancel: cancel}, nil
}
