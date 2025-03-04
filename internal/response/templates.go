package response

import (
	"bytes"
	"html/template"
	"net/http"
	"path/filepath"
	"strings"

	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
)

// TemplateCache holds the parsed templates
type TemplateCache map[string]*template.Template

// NewTemplateCache creates a new cache of parsed templates
func NewTemplateCache() (TemplateCache, error) {
	cache := make(TemplateCache)

	pages, err := assets.EmbeddedFiles.ReadDir("templates/pages")
	if err != nil {
		return nil, err
	}

	for _, page := range pages {
		name := page.Name()

		// Skip directories
		if page.IsDir() {
			subpages, err := assets.EmbeddedFiles.ReadDir("templates/pages/" + name)
			if err != nil {
				return nil, err
			}
			for _, subpage := range subpages {
				if !subpage.IsDir() {
					patterns := []string{
						"templates/base.html",
						"templates/partials/*.html",
						filepath.Join("templates/pages", name, subpage.Name()),
					}

					ts, err := template.New(name+"/"+subpage.Name()).Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
					if err != nil {
						return nil, err
					}

					cache[name+"/"+subpage.Name()] = ts
				}
			}
			continue
		}

		patterns := []string{
			"templates/base.html",
			"templates/partials/*.html",
			filepath.Join("templates/pages", name),
		}

		ts, err := template.New(name).Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
		if err != nil {
			return nil, err
		}

		cache[name] = ts
	}

	return cache, nil
}

func Partial(w http.ResponseWriter, status int, data any, partialPath string) error {
	return PartialWithHeaders(w, status, data, nil, partialPath)
}

func PartialWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, partialPath string) error {
	// For partials, we only need the partial template and any shared partials
	patterns := []string{"partials/*.html", partialPath}

	// Get the template name from the path
	// e.g., "templates/partials/messages_list.html" -> "messages_list"
	templateName := strings.TrimSuffix(filepath.Base(partialPath), ".html")

	return NamedTemplateWithHeaders(w, status, data, headers, templateName, patterns...)
}

func Page(w http.ResponseWriter, status int, data any, pagePath string) error {
	return PageWithHeaders(w, status, data, nil, pagePath)
}

func PageWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, pagePath string) error {
	patterns := []string{"base.html", "partials/*.html", pagePath}

	return NamedTemplateWithHeaders(w, status, data, headers, "base", patterns...)
}

// func PageWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, pagePath string) error {
// 	patterns := []string{"base.html", "partials/*.html", pagePath}
// 	templateName := strings.TrimSuffix(filepath.Base(pagePath), ".html")
// 	return NamedTemplateWithHeaders(w, status, data, headers, templateName, patterns...)
// }

func NamedTemplate(w http.ResponseWriter, status int, data any, templateName string, patterns ...string) error {
	return NamedTemplateWithHeaders(w, status, data, nil, templateName, patterns...)
}

func NamedTemplateWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, templateName string, patterns ...string) error {
	// Ensure patterns point to the correct directory
	for i := range patterns {
		patterns[i] = "templates/" + patterns[i]
	}

	// Parse the templates
	ts, err := template.New("").Funcs(funcs.TemplateFuncs).ParseFS(assets.EmbeddedFiles, patterns...)
	if err != nil {
		// Log or return the error to the caller
		return err
	}

	// Prepare a buffer for the output
	buf := new(bytes.Buffer)
	if err = ts.ExecuteTemplate(buf, templateName, data); err != nil {
		// Log or return the error to the caller
		return err
	}

	// Add headers and write response
	for key, value := range headers {
		w.Header()[key] = value
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err = buf.WriteTo(w)

	return err
}

// RenderOptions allows configuring how templates are rendered
type RenderOptions struct {
	Layout  string      // Optional layout template (e.g., "base")
	Headers http.Header // Optional additional headers
}

// Render handles both partial and full-page rendering based on options
func Render(w http.ResponseWriter, status int, data any, templatePath string, opts *RenderOptions) error {
	if opts == nil {
		opts = &RenderOptions{}
	}

	patterns := []string{"partials/*.html", templatePath}

	// If a layout is specified, include it and use it as the entry point
	templateName := strings.TrimSuffix(filepath.Base(templatePath), ".html")
	if opts.Layout != "" {
		patterns = append([]string{opts.Layout + ".html"}, patterns...)
		templateName = opts.Layout
	}

	return NamedTemplateWithHeaders(w, status, data, opts.Headers, templateName, patterns...)
}
