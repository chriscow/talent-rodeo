package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

const remoteGEOID = "103644278"

type JobListing struct {
	Position       string `json:"job_position"`
	Link           string `json:"job_link"`
	ID             string `json:"job_id"`
	CompanyName    string `json:"company_name"`
	CompanyProfile string `json:"company_profile"`
	Location       string `json:"job_location"`
	PostingDate    string `json:"job_posting_date"`
	CompanyLogo    string `json:"company_logo_url"`
}

// For parsing the entire array of job listings
type JobListings []JobListing

type JobDetail struct {
	Position          string       `json:"job_position"`
	Status            string       `json:"jobs_status"`
	Location          string       `json:"job_location"`
	CompanyName       string       `json:"company_name"`
	CompanyLinkedInID string       `json:"company_linkedin_id"`
	PostingTime       string       `json:"job_posting_time"`
	BasePay           string       `json:"base_pay"`
	Description       string       `json:"job_description"`
	SeniorityLevel    string       `json:"Seniority_level"`
	EmploymentType    string       `json:"Employment_type"`
	JobFunction       string       `json:"Job_function"`
	Industries        string       `json:"Industries"`
	Recruiters        []Recruiter  `json:"recruiter_details"`
	SimilarJobs       []RelatedJob `json:"similar_jobs"`
	PeopleAlsoViewed  []RelatedJob `json:"people_also_viewed"`
}

type Recruiter struct {
	Photo      string `json:"recruiter_photo"`
	Name       string `json:"recruiter_name"`
	Title      string `json:"recruiter_title"`
	ProfileURL string `json:"recruiter_profile_url"`
}

type RelatedJob struct {
	Position    string `json:"job_position"`
	Company     string `json:"job_company"`
	Location    string `json:"job_location"`
	PostingTime string `json:"job_posting_time"`
	Link        string `json:"job_link"`
}

// For parsing the array of job listings
type JobDetails []JobDetail

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal(err)
	}

	if os.Getenv("SCRAPINGDOG_API_KEY") == "" {
		log.Fatal("SCRAPINGDOG_API_KEY is required")
	}

	req, err := http.NewRequest("GET", "https://api.scrapingdog.com/linkedinjobs/", nil)
	if err != nil {
		log.Fatal(err)
	}

	q := req.URL.Query()
	q.Add("api_key", os.Getenv("SCRAPINGDOG_API_KEY"))
	q.Add("field", "golang")
	q.Add("geoid", remoteGEOID)
	q.Add("page", "2")
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("unexpected status code: %d", resp.StatusCode)
	}

	var jobListings JobListings
	if err := json.NewDecoder(resp.Body).Decode(&jobListings); err != nil {
		log.Fatal(err)
	}

	for _, job := range jobListings {
		log.Printf("Position: %s\nLink: %s\nID: %s\nCompany Name: %s\nCompany Profile: %s\nLocation: %s\nPosting Date: %s\nCompany Logo: %s\n\n",
			job.Position, job.Link, job.ID, job.CompanyName, job.CompanyProfile, job.Location, job.PostingDate, job.CompanyLogo)
	}

	// curl "https://api.scrapingdog.com/linkedinjobs?api_key=5eaa61a6e562fc52fe763tr516e4653&job_id=3539667535"
	/*
		jobID1 := "4117956991"

		req, err := http.NewRequest("GET", "https://api.scrapingdog.com/linkedinjobs/", nil)
		if err != nil {
			log.Fatal(err)
		}

		q := req.URL.Query()
		q.Add("api_key", os.Getenv("SCRAPINGDOG_API_KEY"))
		q.Add("job_id", jobID1)
		req.URL.RawQuery = q.Encode()

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Fatalf("unexpected status code: %d", resp.StatusCode)
		}

		var jobDetails JobDetails
		if err := json.NewDecoder(resp.Body).Decode(&jobDetails); err != nil {
			log.Fatal(err)
		}

		println("\n\n----------------- Job Details -----------------\n\n")
		for _, job := range jobDetails {
			log.Printf("Position: %s\nStatus: %s\nLocation: %s\nCompany Name: %s\nCompany LinkedIn ID: %s\nPosting Time: %s\nBase Pay: %s\nDescription: %s\nSeniority Level: %s\nEmployment Type: %s\nJob Function: %s\nIndustries: %s\nRecruiters: %+v\nSimilar Jobs: %+v\nPeople Also Viewed: %+v\n\n",
				job.Position, job.Status, job.Location, job.CompanyName, job.CompanyLinkedInID, job.PostingTime, job.BasePay, job.Description, job.SeniorityLevel, job.EmploymentType, job.JobFunction, job.Industries, job.Recruiters, job.SimilarJobs, job.PeopleAlsoViewed)
			println("\n\n")
		}
	*/
}
