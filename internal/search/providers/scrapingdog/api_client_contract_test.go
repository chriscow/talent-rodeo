package scrapingdog

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"talent-rodeo/internal"
	"testing"
)

func TestAPIClientJobsEndpointAndWorkType(t *testing.T) {
	requests := make(chan *http.Request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("job_id") != "" {
			if _, err := io.WriteString(w, `[{"job_position":"Engineer"}]`); err != nil {
				t.Errorf("write overview response: %v", err)
			}
			return
		}
		if _, err := io.WriteString(w, `[]`); err != nil {
			t.Errorf("write search response: %v", err)
		}
	}))
	defer server.Close()

	client := NewAPIClient("test-key", WithAPIBaseURL(server.URL+"/jobs"), WithAPILogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if _, err := client.SearchJobListings(context.Background(), "engineer", "92000000", internal.WorkTypeRemote, internal.ExperienceLevelAssociate, 1, SortByDay); err != nil {
		t.Fatalf("SearchJobListings() error = %v", err)
	}
	if _, err := client.GetJobDetails(context.Background(), "123"); err != nil {
		t.Fatalf("GetJobDetails() error = %v", err)
	}
	search := (<-requests).URL
	if search.Path != "/jobs" || search.Query().Get("work_type") != "remote" || search.Query().Has("job_type") {
		t.Errorf("search URL = %s, want /jobs with work_type=remote and no job_type", search)
	}
	if details := (<-requests).URL; details.Path != "/jobs" || details.Query().Get("job_id") != "123" {
		t.Errorf("overview URL = %s, want /jobs with job_id=123", details)
	}
}
