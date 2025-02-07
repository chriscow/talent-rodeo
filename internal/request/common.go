package request

import (
	"net/http"
)

// ScheduleRequest represents the common scheduling request structure
type ScheduleRequest struct {
	DateTime string `json:"datetime" form:"datetime"`
	Timezone string `json:"timezone" form:"timezone"`
}

// DecodeScheduleRequest decodes a schedule request from either JSON or form data
func DecodeScheduleRequest(w http.ResponseWriter, r *http.Request) (*ScheduleRequest, error) {
	var req ScheduleRequest

	if r.Header.Get("Content-Type") == "application/json" {
		if err := DecodeJSON(w, r, &req); err != nil {
			return nil, err
		}
	} else {
		if err := DecodePostForm(r, &req); err != nil {
			return nil, err
		}
	}

	return &req, nil
}

// IsHTMXRequest checks if the request is from HTMX
func IsHTMXRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}
