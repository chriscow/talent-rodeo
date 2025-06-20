package nudge

import (
	"fmt"
	"html/template"
	"path/filepath"
	"sync"
	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
)

var (
	templateCache = make(map[NudgeType]*template.Template)
	once          sync.Once
)

// LoadAndCacheTemplates parses and caches all nudge email templates from the embedded filesystem.
// It should be called once on application startup.
func LoadAndCacheTemplates() error {
	var initErr error
	once.Do(func() {
		for _, nudgeType := range AllNudgeTypes {
			templatePath := nudgeType.EmailTemplatePath()
			if templatePath == "" {
				continue // Skip nudge types without a template path
			}

			t, err := template.New(filepath.Base(templatePath)).Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, templatePath)
			if err != nil {
				initErr = fmt.Errorf("failed to parse template for %s (%s): %w", nudgeType, templatePath, err)
				return
			}
			templateCache[nudgeType] = t
		}
	})
	return initErr
}

// GetTemplate retrieves a pre-parsed template from the cache.
func GetTemplate(nudgeType NudgeType) (*template.Template, error) {
	t, ok := templateCache[nudgeType]
	if !ok {
		return nil, fmt.Errorf("template not found in cache for nudge type: %s", nudgeType)
	}
	return t, nil
}
