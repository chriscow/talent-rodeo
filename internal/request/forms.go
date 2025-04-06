package request

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-playground/form/v4"
)

var decoder = form.NewDecoder()

func DecodeForm(r *http.Request, dst any) error {
	err := r.ParseForm()
	if err != nil {
		return err
	}

	return decodeURLValues(r.Form, dst)
}

// DecodePostForm decodes the request body into the destination struct.
// It expects the request to have a Content-Type of application/x-www-form-urlencoded.
func DecodePostForm(r *http.Request, dst any) error {
	// Parse the form data
	err := r.ParseForm()
	if err != nil {
		return err
	}

	// Try to parse as multipart form if the content type is multipart/form-data
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		err = r.ParseMultipartForm(10 << 20) // 10 MB max
		if err != nil {
			return err
		}
	}

	return decoder.Decode(dst, r.PostForm)
}

func DecodeQueryString(r *http.Request, dst any) error {
	return decodeURLValues(r.URL.Query(), dst)
}

func decodeURLValues(v url.Values, dst any) error {
	err := decoder.Decode(dst, v)
	if err != nil {
		var invalidDecoderError *form.InvalidDecoderError

		if errors.As(err, &invalidDecoderError) {
			panic(err)
		}
	}

	return err
}
