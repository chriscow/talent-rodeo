package rodeo

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/chriscow/minds"
)

/*
	type CandidateInfo struct {
		// KnownFacts are facts about the candidate that are known or discovered during the call
		KnownFacts          []string        `json:"known_facts" description:"Facts about the candidate that are known or discovered during the call"`
		InferredPreferences []string        `json:"inferred_preferences" description:"Career preferences inferred from the candidate's responses"`
		PotentialConcerns   []string        `json:"potential_concerns" description:"Potential concerns about the candidate"`
		EngagementLevel     EngagementLevel `json:"engagement_level" enum:"high,medium,low"  description:"The candidate's engagement level during the call"`
		Resume              string          `json:"resume" description:"The candidate's resume in markdown format"`
		Name                string          `json:"name" description:"The candidate's name"`
		Email               string          `json:"email" description:"The candidate's email address"`
		PhoneNumber         string          `json:"phone_number" description:"The candidate's phone number"`
		Timezone            string          `json:"timezone" description:"The candidate's timezone"`
	}
*/
type AddKnownFactArgs struct {
	Facts []string `json:"fact" description:"Additional new facts learned about the candidate"`
}

type addKnownFactsTool struct {
	state *ConversationState
}

func (t *addKnownFactsTool) Type() string { return "function" }
func (t *addKnownFactsTool) Name() string { return "add_known_fact" }
func (t *addKnownFactsTool) Description() string {
	return "Add newly discovered facts about the candidate"
}

func (t *addKnownFactsTool) Parameters() minds.Definition {
	def, _ := minds.GenerateSchema(AddKnownFactArgs{})
	return *def
}

func (t *addKnownFactsTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	var args AddKnownFactArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, err
	}
	t.state.Candidate.KnownFacts = append(t.state.Candidate.KnownFacts, args.Facts...)

	sb := strings.Builder{}
	sb.WriteString("Fact(s) known about the candidate:\n")
	for _, fact := range args.Facts {
		sb.WriteString(fact)
		sb.WriteString("\n")
	}
	return []byte(sb.String()), nil
}

type AddInferredPreferenceArgs struct {
	Preferences []string `json:"preference" description:"Additional discovered career preferences inferred from the candidate's responses"`
}

type addInferredPreferenceTool struct {
	state *ConversationState
}

func (t *addInferredPreferenceTool) Type() string { return "function" }
func (t *addInferredPreferenceTool) Name() string { return "add_inferred_preference" }
func (t *addInferredPreferenceTool) Description() string {
	return "Add a career preference inferred from the candidate's responses"
}

func (t *addInferredPreferenceTool) Parameters() minds.Definition {
	def, _ := minds.GenerateSchema(AddInferredPreferenceArgs{})
	return *def
}

func (t *addInferredPreferenceTool) Call(ctx context.Context, input []byte) ([]byte, error) {
	var args AddInferredPreferenceArgs
	if err := json.Unmarshal(input, &args); err != nil {
		return nil, err
	}
	t.state.Candidate.InferredPreferences = append(t.state.Candidate.InferredPreferences, args.Preferences...)

	sb := strings.Builder{}
	sb.WriteString("Inferred career preferences known:\n")
	for _, pref := range args.Preferences {
		sb.WriteString(pref)
		sb.WriteString("\n")
	}
	return []byte(sb.String()), nil
}
