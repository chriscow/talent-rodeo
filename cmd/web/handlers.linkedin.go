package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"talent-rodeo/internal/response"
	"talent-rodeo/internal/scrapingdog/linkedin"
	"talent-rodeo/internal/utils"

	"github.com/alexedwards/flow"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
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
	searchManager := linkedin.NewSearchManager(app.db.DB, app.logger)

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

	searchManager := linkedin.NewSearchManager(app.db.DB, app.logger)

	// Get the job details from your database
	job, err := searchManager.GetJobDetail(r.Context(), jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	requirements, err := searchManager.GetJobRequirements(r.Context(), jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Job"] = job
	data["Description"] = template.HTML(job.Description)
	data["Requirements"] = requirements

	opts := &response.RenderOptions{}
	if r.Header.Get("HX-Request") != "true" {
		opts.Layout = "base"
	}

	if err := response.Render(w, http.StatusOK, data, "pages/job_detail.tmpl", opts); err != nil {
		app.serverError(w, r, err)
	}
}

// Gets the partial HTML for the job match page
func (app *application) handleJobMatch(w http.ResponseWriter, r *http.Request) {
	jobID := flow.Param(r.Context(), "id")

	searchManager := linkedin.NewSearchManager(app.db.DB, app.logger)

	// Get the job details from your database
	jobDetail, err := searchManager.GetJobDetail(r.Context(), jobID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	/*
		type JobDetail struct {
			Position          string       `json:"job_position" db:"position"`
			Status            string       `json:"job_status" db:"status"`
			Location          string       `json:"job_location" db:"location"`
			CompanyName       string       `json:"company_name" db:"company_name"`
			CompanyLinkedInID string       `json:"company_linkedin_id" db:"company_linkedin_id"`
			PostingTime       string       `json:"job_posting_time" db:"posting_time"`
			BasePay           string       `json:"base_pay" db:"base_pay"`
			Description       string       `json:"job_description" db:"description"`
			SeniorityLevel    string       `json:"seniority_level" db:"seniority_level"`
			EmploymentType    string       `json:"employment_type" db:"employment_type"`
			JobFunction       string       `json:"job_function" db:"job_function"`
			Industries        string       `json:"industries" db:"industries"`
			Recruiters        []Recruiter  `json:"recruiter_details" db:"recruiter_details"`
			SimilarJobs       []RelatedJob `json:"similar_jobs" db:"similar_jobs"`
			PeopleAlsoViewed  []RelatedJob `json:"people_also_viewed" db:"people_also_viewed"`
		}
	*/

	match := linkedin.JobMatch{}

	config := linkedin.MatchConfig{
		DetailLevel:        "standard",
		RequiredConfidence: 0.8,
		WeightRequired:     1.0,
		WeightPreferred:    0.5,
		IncludeVisuals:     true,
		VerificationAge:    30,
	}
	match.AnalyzeMatch(config)

	// Populate the match data from the job details
	match.Position.Title = jobDetail.Position
	match.Position.Company = jobDetail.CompanyName
	match.Position.Location = jobDetail.Location
	match.Position.Type = jobDetail.EmploymentType
	match.Position.Level = jobDetail.SeniorityLevel

	// Parse the job description using your LLM to extract qualifications
	qualifications, err := app.analyzeJobDescription(r.Context(), jobDetail.Description)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	match.Qualifications = qualifications

	// Perform the match analysis
	if err := match.AnalyzeMatch(config); err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data["Match"] = match

	if err := response.Partial(w, http.StatusOK, data, "partials/job_match.tmpl"); err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) analyzeJobDescription(ctx context.Context, jobDesc string) ([]linkedin.Qualification, error) {
	prompt, _ := utils.CreateTemplate("prompts/job-match-analysis.txt")

	llm, err := openai.NewProvider()
	if err != nil {
		return nil, err
	}

	jobDesc = fmt.Sprintf("[Job Description]\n%s", jobDesc)
	// resume = fmt.Sprintf("[Candidate Resume]\n%s", resume)
	// transcript = fmt.Sprintf("[Transcript Summary]\n%s", transcript)

	type qualifications struct {
		Qualifications []linkedin.Qualification `json:"qualifications" description:"Qualification represents a single required or preferred qualification which could be a skill, experience, certification, or educational requirement"`
	}
	output := qualifications{}

	schema, err := minds.NewResponseSchema("qualifications", "Extracts a list of qualifications details from a job description", output)

	b, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return nil, err
	}

	fmt.Println(string(b))

	req := minds.NewRequest(minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: prompt.Body(),
		},
		{
			Role:    minds.RoleUser,
			Content: jobDesc,
		},
	}, minds.WithResponseSchema(*schema))

	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, err
	}

	body := resp.String()
	if err := json.Unmarshal([]byte(body), &output); err != nil {
		return nil, err
	}

	return output.Qualifications, nil
}
