package vapi_call

import (
	"context"
	"fmt"
	"log"
	"os"
	"talent-rodeo/cmd/try"
	"talent-rodeo/internal/utils"
	"talent-rodeo/internal/vapi"

	"github.com/chriscow/minds"
	"github.com/spf13/cobra"
)

var (
	// Flag variables
	systemPromptFile   string
	systemPromptString string
	assistantID        string
	outboundPhoneID    string
	phoneNumber        string
	candidateName      string
	resumeFile         string
)

// callCmd represents the call command
var callCmd = &cobra.Command{
	Use:   "call",
	Short: "Make an outbound call using VAPI",
	Long: `Make an outbound call using VAPI with configurable parameters.
Example:
  go run ./cmd/ call
  try call --prompt-file prompts/interview.txt --phone "+1234567890" --name "John Doe"`,
	RunE: runCall,
}

func init() {
	try.TryCmd.AddCommand(callCmd)

	// Add flags
	callCmd.Flags().StringVar(&systemPromptFile, "prompt-file", "prompts/recruiter.system.006.txt", "Path to the system prompt file")
	callCmd.Flags().StringVar(&systemPromptString, "prompt", "", "System prompt string (overrides prompt-file)")
	callCmd.Flags().StringVar(&assistantID, "assistant-id", "30709dea-4c26-457a-96a0-4ee7488f6b72", "VAPI Assistant ID")
	callCmd.Flags().StringVar(&outboundPhoneID, "phone-id", "eb8df153-8754-417e-b4e3-580638f95355", "Outbound Phone ID")
	callCmd.Flags().StringVar(&phoneNumber, "phone", "+14258295443", "Phone number to call")
	callCmd.Flags().StringVar(&candidateName, "name", "Chris Cowherd", "Candidate name")
	callCmd.Flags().StringVar(&resumeFile, "resume", "./assets/resumes/chris-cowherd.yaml", "Path to the resume text file")
}

func runCall(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Read resume file if provided
	var resumeContent string
	if resumeFile != "" {
		content, err := os.ReadFile(resumeFile)
		if err != nil {
			return fmt.Errorf("failed to read resume file: %w", err)
		}
		resumeContent = string(content)
	}

	// Get system prompt either from file or flag
	var systemPrompt string
	if systemPromptString != "" {
		systemPrompt = systemPromptString
	} else if systemPromptFile != "" {
		prompt, err := utils.CreateTemplate(systemPromptFile)
		if err != nil {
			return fmt.Errorf("failed to load prompt file: %w", err)
		}

		// Execute template with resume data
		systemPrompt, err = prompt.Execute(map[string]string{
			"Resume": resumeContent,
		})
		if err != nil {
			return fmt.Errorf("failed to execute prompt template: %w", err)
		}
	} else {
		return fmt.Errorf("either --prompt or --prompt-file must be specified")
	}

	modelConfig := vapi.DefaultModelConfig
	modelConfig.Messages = []vapi.ModelMessage{{
		Role:    "system",
		Content: systemPrompt,
	}}

	analysisPrompt := `
You are an AI assistant tasked with analyzing a transcribed interview call between a job candidate and Shirley, an AI job search specialist from Talent Rodeo. Based on the full transcript provided, carefully extract and structure the candidate's preferences and keywords into a concise JSON format to guide an accurate job search.

### Guidelines for Extraction and Structuring:

- **WorkLocation:**
  - Clearly select "Remote" only if explicitly stated by the candidate.
  - If location is specified, match it precisely to one of the following largest U.S. metropolitan areas:
    - Greater New York City area
    - Greater Los Angeles area
    - Greater Chicago area
    - Greater Dallas-Fort Worth area
    - Greater Houston area
    - Greater Washington D.C. area
    - Greater Miami area
    - Greater Philadelphia area
    - Greater Atlanta area
    - Greater Boston area
    - Greater San Francisco Bay area
    - Greater Phoenix area
    - Greater Riverside-San Bernardino area
    - Greater Detroit area
    - Greater Seattle area
    - Greater Minneapolis-St. Paul area
    - Greater San Diego area
    - Greater Tampa area
    - Greater Denver area
    - Greater Baltimore area

- **Searches:**
  - Include only job titles, roles, specific skills, or technologies explicitly highlighted by the candidate.
  - Do not infer or generalize terms; use exact phrases or keywords mentioned during the conversation.
  - List terms clearly and distinctly for effective use as OR search criteria.

- **FirstPersonPrompt:**
  - Compose this prompt entirely in the first person as the candidate, summarizing their preferences and priorities for evaluating job descriptions.
  - Include key considerations such as role, responsibilities, desired environment, career goals, and any important aspects discussed explicitly during the call.

Ensure your JSON output accurately represents the candidate's stated preferences and maintains fidelity to their exact language and expressions from the call transcript.
`

	analysisSchema, err := minds.GenerateSchema(CandidateInfo{})
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	log.Printf("Making call to %s (%s)...", phoneNumber, candidateName)

	result, err := vapi.CreateCall(ctx, vapi.Call{
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
			Number: phoneNumber,
			Name:   candidateName,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create call: %w", err)
	}

	log.Printf("Call created successfully with ID: %s", result.ID)
	log.Printf("Call status: %s", result.Status)
	if result.Monitor.ListenURL != "" {
		log.Printf("Listen URL: %s", result.Monitor.ListenURL)
	}
	if result.Monitor.ControlURL != "" {
		log.Printf("Control URL: %s", result.Monitor.ControlURL)
	}

	return nil
}

// CandidateInfo represents the structured data we want to extract from the call
type CandidateInfo struct {
	WorkLocation      string   `json:"work_location"`
	SearchKeywords    []string `json:"search_keywords"`
	FirstPersonPrompt string   `json:"first_person_prompt"`

	// Name                string   `json:"name"`
	// CurrentRole         string   `json:"current_role"`
	// YearsOfExperience   float64  `json:"years_of_experience"`
	// KeySkills           []string `json:"key_skills"`
	// PreferredRoles      []string `json:"preferred_roles"`
	// SalaryExpectations  string   `json:"salary_expectations"`
	// LocationPreferences []string `json:"location_preferences"`
	// NoticePeriod        string   `json:"notice_period"`
	// Availability        string   `json:"availability"`
	// Notes               string   `json:"notes"`
}
