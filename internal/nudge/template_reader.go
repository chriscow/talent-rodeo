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
	promptPath := nudgeType.PromptTemplatePath()
	if promptPath == "" {
		return nil, fmt.Errorf("no template path for nudge type: %s", nudgeType)
	}

	template, err := assets.EmbeddedFiles.ReadFile(promptPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template %s: %w", promptPath, err)
	}

	return template, nil
}
