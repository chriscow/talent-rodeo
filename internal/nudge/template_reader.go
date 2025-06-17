package nudge

import (
	"fmt"
	"talent-rodeo/assets"
)

// AssetTemplateReader implements TemplateReader using embedded assets
type AssetTemplateReader struct{}

// NewAssetTemplateReader creates a new template reader for embedded assets
func NewAssetTemplateReader() *AssetTemplateReader {
	return &AssetTemplateReader{}
}

// ReadTemplate reads a template file for the given nudge type
func (r *AssetTemplateReader) ReadTemplate(nudgeType NudgeType) ([]byte, error) {
	promptPath := getPromptPath(nudgeType)
	if promptPath == "" {
		return nil, fmt.Errorf("no template path for nudge type: %s", nudgeType)
	}

	template, err := assets.EmbeddedFiles.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", promptPath, err)
	}

	return template, nil
}

// getPromptPath returns the file path for the nudge type's prompt template
func getPromptPath(nudgeType NudgeType) string {
	switch nudgeType {
	case NudgeTypeNoResume:
		return "prompts/shirely-nudge-no-resume.txt"
	case NudgeTypeNoCall:
		return "prompts/shirely-nudge-no-call.txt"
	case NudgeTypeFiveStarMatches:
		return "prompts/shirely-nudge-five-star-matches.txt"
	default:
		return ""
	}
}
