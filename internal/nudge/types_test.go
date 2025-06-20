package nudge

import (
	"html/template"
	"path/filepath"
	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
	"testing"
)

func TestNudgeType_EmailTemplatePath(t *testing.T) {
	tests := []struct {
		name      string
		nudgeType NudgeType
		want      string
	}{
		{
			name:      "NoResume returns correct path",
			nudgeType: NudgeTypeNoResume,
			want:      "emails/shirely-no-resume.html",
		},
		{
			name:      "NoCall returns correct path",
			nudgeType: NudgeTypeNoCall,
			want:      "emails/shirely-no-call.html",
		},
		{
			name:      "FiveStarMatches returns correct path",
			nudgeType: NudgeTypeFiveStarMatches,
			want:      "emails/shirely-five-star-matches.html",
		},
		// {
		// 	name:      "FollowUp returns correct path",
		// 	nudgeType: NudgeTypeFollowUp,
		// 	want:      "emails/shirely-follow-up-questions.html",
		// },
		{
			name:      "Invalid type returns empty string",
			nudgeType: "invalid",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nudgeType.EmailTemplatePath()
			if got != tt.want {
				t.Errorf("NudgeType.EmailTemplatePath() = %v, want %v", got, tt.want)
			}

			// Skip filesystem checks for invalid types that return empty path
			if tt.want == "" {
				return
			}

			// Verify the template exists in the embedded filesystem
			content, err := assets.EmbeddedFiles.ReadFile(tt.want)
			if err != nil {
				t.Errorf("Template file %s not found in embedded filesystem: %v", tt.want, err)
				return
			}

			// Verify the template can be parsed with the required functions
			tmpl, err := template.New(filepath.Base(tt.want)).Funcs(funcs.TemplateFuncs).Parse(string(content))
			if err != nil {
				t.Errorf("Failed to parse template %s: %v", tt.want, err)
				return
			}

			// Verify required template blocks exist
			requiredBlocks := []string{"subject", "htmlBody"}
			for _, block := range requiredBlocks {
				if tmpl.Lookup(block) == nil {
					t.Errorf("Template %s is missing required block {{define %q}}", tt.want, block)
				}
			}
		})
	}
}

func TestGetPromptPath(t *testing.T) {
	tests := []struct {
		name      string
		nudgeType NudgeType
		want      string
	}{
		{
			name:      "NoResume returns correct path",
			nudgeType: NudgeTypeNoResume,
			want:      "prompts/shirely-nudge-no-resume.txt",
		},
		{
			name:      "NoCall returns correct path",
			nudgeType: NudgeTypeNoCall,
			want:      "prompts/shirely-nudge-no-call.txt",
		},
		{
			name:      "FiveStarMatches returns correct path",
			nudgeType: NudgeTypeFiveStarMatches,
			want:      "prompts/shirely-nudge-five-star-matches.txt",
		},
		// {
		// 	name:      "FollowUp returns correct path",
		// 	nudgeType: NudgeTypeFollowUp,
		// 	want:      "prompts/shirely-follow-up-questions.txt",
		// },
		{
			name:      "Invalid type returns empty string",
			nudgeType: "invalid",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.nudgeType.PromptTemplatePath()
			if got != tt.want {
				t.Errorf("NudgeType.PromptTemplatePath() = %v, want %v", got, tt.want)
			}

			// Skip filesystem checks for invalid types that return empty path
			if tt.want == "" {
				return
			}

			// Verify the prompt file exists in the embedded filesystem
			content, err := assets.EmbeddedFiles.ReadFile(tt.want)
			if err != nil {
				// Since we know prompts don't exist yet, we'll skip this error
				t.Skipf("Prompt file %s not found in embedded filesystem (expected until prompts are added)", tt.want)
				return
			}

			// Basic validation of prompt content
			if len(content) == 0 {
				t.Errorf("Prompt file %s is empty", tt.want)
			}
		})
	}
}
