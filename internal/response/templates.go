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

func Partial(w http.ResponseWriter, status int, data any, partialPath string) error {
	return PartialWithHeaders(w, status, data, nil, partialPath)
}

func PartialWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, partialPath string) error {
	// For partials, we only need the partial template and any shared partials
	patterns := []string{"partials/*.tmpl", partialPath}

	// Get the template name from the path
	// e.g., "templates/partials/messages_list.tmpl" -> "messages_list"
	templateName := strings.TrimSuffix(filepath.Base(partialPath), ".tmpl")

	return NamedTemplateWithHeaders(w, status, data, headers, templateName, patterns...)
}

func Page(w http.ResponseWriter, status int, data any, pagePath string) error {
	return PageWithHeaders(w, status, data, nil, pagePath)
}

func PageWithHeaders(w http.ResponseWriter, status int, data any, headers http.Header, pagePath string) error {
	patterns := []string{"base.tmpl", "partials/*.tmpl", pagePath}

	return NamedTemplateWithHeaders(w, status, data, headers, "base", patterns...)
}

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
	w.WriteHeader(status)
	_, err = buf.WriteTo(w)

	return err
}
