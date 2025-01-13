package main

import (
	"net/http"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/scrapingdog/linkedin"

	"github.com/alexedwards/flow"
)

// func (app *application) linkedInSearch(w http.ResponseWriter, r *http.Request) {

// 	data := app.newTemplateData(r)

// 	if err := response.Page(w, http.StatusOK, data, "pages/search_jobs.tmpl"); err != nil {
// 		app.serverError(w, r, err)
// 	}
// }

func (app *application) linkedInSearch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		if r.Header.Get("HX-Request") == "true" {
			// This is an HTMX search request
			app.handleSearch(w, r)
		} else {
			data := app.newTemplateData(r)

			if err := response.Page(w, http.StatusOK, data, "pages/search_jobs.tmpl"); err != nil {
				app.serverError(w, r, err)
			}
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (app *application) handleSearch(w http.ResponseWriter, r *http.Request) {
	searchManager := linkedin.NewSearchManager(app.db.DB)

	keyword := r.FormValue("keyword")
	geoId := r.FormValue("geoId")

	var opts []linkedin.JobSearchOption

	// Add search options if provided
	if sortBy := r.FormValue("sortBy"); sortBy != "" {
		opts = append(opts, linkedin.WithSortBy(sortBy))
	}
	if jobType := r.FormValue("jobType"); jobType != "" {
		opts = append(opts, linkedin.WithJobType(jobType))
	}
	if expLevel := r.FormValue("expLevel"); expLevel != "" {
		opts = append(opts, linkedin.WithExpLevel(expLevel))
	}
	if workType := r.FormValue("workType"); workType != "" {
		opts = append(opts, linkedin.WithWorkType(workType))
	}

	jobs, err := searchManager.SearchJobs(r.Context(), keyword, geoId, opts...)

	data := app.newTemplateData(r)
	data["Jobs"] = jobs
	data["Error"] = err

	if err != nil {
		data["Error"] = err.Error()
	}

	if err := response.Partial(w, http.StatusOK, data, "partials/job_results.tmpl"); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (app *application) jobDetail(w http.ResponseWriter, r *http.Request) {
	jobID := flow.Param(r.Context(), "id")
	// jobID := strings.TrimPrefix(r.URL.Path, "/jobs/")

	searchManager := linkedin.NewSearchManager(app.db.DB)

	// Get the job details from your database
	job, err := searchManager.GetJobDetail(r.Context(), jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Job"] = job

	if err := response.Page(w, http.StatusOK, data, "pages/job_detail.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}
