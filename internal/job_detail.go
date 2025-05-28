package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"talent-rodeo/assets"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds/tools"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	JobDetailsCollection      = "job_details"
	JobCriteriaLinkCollection = "job_criteria_links"
)

type SearchQueue interface {
	QueueSearch(ctx context.Context, search SearchTask) error
	GetQueueDepth(ctx context.Context) (int, error)
}

// JobDetail contains comprehensive information about a specific job.
// This is returned by GetJobDetails and includes all available
// information about the position.
type JobDetail struct {
	// Basic job information (duplicated from JobListing for clarity)
	ID            string
	Provider      string
	ProviderJobID string
	Title         string
	Company       string
	Location      string
	PostedDate    time.Time
	URL           string
	ApplyURL      string // Direct application link, if different from job URL

	// Detailed information
	Description string // Complete job description

	Requirements   []string // List of job requirements
	Qualifications []string // Required qualifications
	Benefits       []string // List of benefits
	Skills         []string // Required skills
	SalaryInfo     *SalaryRange
	EmploymentType string // Full-time, part-time, contract, etc.
	WorkplaceType  string // Remote, on-site, hybrid
	Industry       string
	CompanyInfo    *Company

	// Additional fields from ScrapingDog
	Status           string
	SeniorityLevel   string
	JobFunction      string
	Recruiters       []Recruiter
	SimilarJobs      []RelatedJob
	PeopleAlsoViewed []RelatedJob

	ExtractedData *JobDetailExtractedData

	CreatedAt    time.Time
	UpdatedAt    *time.Time
	SearchJobIDs []string
}

type JobDetailCriteriaLink struct {
	CriteriaID string
	JobID      string
	CreatedAt  time.Time
}

type JobDetailExtractedData struct {
	Summary             string   `json:"summary" description:"Core function and scope of the role"`
	Responsibilities    []string `json:"responsibilities" description:"Specific tasks the hire will perform"`
	Requirements        []string `json:"requirements" description:"Mandatory barriers to entry (e.g., degrees, legal requirements)"`
	Skills              []string `json:"skills" description:"Technical/hard skills (tools, languages, systems)"`
	Qualifications      []string `json:"qualifications" description:"Broader competencies (experience level, soft skills, industry knowledge)"`
	BonusQualifications []string `json:"bonus_qualifications" description:"Secondary assets (e.g., familiarity with adjacent technologies)"`
}

// Company contains information about the company posting the job.
// This information may be limited depending on the provider.
type Company struct {
	Name        string
	Website     string
	Industry    string
	Size        string // Company size range
	Description string
	LogoURL     string
	ProfileURL  string
	// Future considerations:
	// - Company culture
	// - Tech stack
	// - Funding status
	// - Growth metrics
}

// SalaryRange represents a salary range for job listings.
// Some providers may not support all fields, so implementations
// should handle partial data appropriately.
type SalaryRange struct {
	Min      float64 `json:"min" description:"The minimum salary in the range"`
	Max      float64 `json:"max" description:"The maximum salary in the range"`
	Currency string  `json:"currency" description:"The currency of the salary range"`
	Period   string  `json:"period" description:"The period of the salary range" enum:"yearly,monthly,hourly"`
	// Future considerations:
	// - Equity compensation
	// - Bonus structure
	// - Benefits value
}

// Recruiter represents a recruiter associated with a job posting
type Recruiter struct {
	Photo      string
	Name       string
	Title      string
	ProfileURL string
}

// RelatedJob represents a related or similar job posting
type RelatedJob struct {
	Title       string
	Company     string
	Location    string
	PostingDate string
	URL         string
}

func (j *JobDetail) GetCacheKey() string {
	return fmt.Sprintf("%s:%s", j.Provider, j.ProviderJobID)
}

func initJobDetail(ctx context.Context, fs *FirestoreClient, job *JobDetail) {
	if job == nil {
		return
	}

	if job.ID == "" {
		job.ID = job.GetCacheKey()
	}

	if job.UpdatedAt == nil && !job.CreatedAt.IsZero() {
		nowUTC := time.Now().UTC()
		job.UpdatedAt = &nowUTC
	}

	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	if job.SearchJobIDs == nil {
		job.SearchJobIDs = []string{}
	}

	job.CreatedAt = time.Now().UTC()
}

func ExtractJobDetailData(ctx context.Context, job *JobDetail) error {
	pb, err := assets.EmbeddedFiles.ReadFile("prompts/job-summary.001.txt")
	if err != nil {
		return fmt.Errorf("failed to read system prompt: %w", err)
	}

	b, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("failed to marshal job detail: %w", err)
	}

	prompt := string(pb) + "\n\n```json\n" + string(b) + "\n```\n\n"

	data, err := tools.StructuredAsk[JobDetailExtractedData](ctx, "extracted-data", prompt,
		tools.WithModel(tools.DeepSeekChat))
	if err != nil {
		return fmt.Errorf("failed to summarize job detail: %w", err)
	}

	job.ExtractedData = &data
	return nil
}

func SaveJobDetail(ctx context.Context, fs *FirestoreClient, job *JobDetail) error {
	initJobDetail(ctx, fs, job)

	_, err := fs.Collection(JobDetailsCollection).Doc(job.ID).Set(ctx, job)
	if err != nil {
		return err
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)
	return nil
}

func SaveJobDetailTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, job *JobDetail) error {
	initJobDetail(ctx, fs, job)

	if err := tx.Set(fs.Collection(JobDetailsCollection).Doc(job.ID), job); err != nil {
		return err
	}

	fs.jobDetailCache.SetWithTTL(job.ID, job, 1, 1*time.Hour)

	return nil
}

func GetJobDetail(ctx context.Context, fs *FirestoreClient, id string) (*JobDetail, bool, error) {
	if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
		return jobDetail, true, nil
	}

	doc, err := fs.Collection(JobDetailsCollection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, false, nil
		}
		return nil, false, err
	}

	var jobDetail JobDetail
	if err := doc.DataTo(&jobDetail); err != nil {
		return nil, false, err
	}

	return &jobDetail, false, nil
}

func LinkJobDetailToCriteria(ctx context.Context, fs *FirestoreClient, criteriaID, jobID string) error {
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	linkRef := fs.Collection(JobCriteriaLinkCollection).Doc(linkID)

	// Check if the link already exists
	doc, err := linkRef.Get(ctx)
	if err == nil && doc.Exists() {
		// Link already exists, skip creation
		return nil
	}

	// Create the link
	link := JobDetailCriteriaLink{
		CriteriaID: criteriaID,
		JobID:      jobID,
		CreatedAt:  time.Now(),
	}

	_, err = linkRef.Set(ctx, link)
	return err
}

// linkJobToCriteria creates a link between a job and criteria
func LinkJobDetailToCriteriaTx(ctx context.Context, fs *FirestoreClient, tx *firestore.Transaction, criteriaID, jobID string) error {
	if criteriaID == "" || jobID == "" {
		return fmt.Errorf("criteriaID and jobID cannot be empty")
	}

	linkID := fmt.Sprintf("%s_%s", criteriaID, jobID)

	linkRef := fs.Collection(JobCriteriaLinkCollection).Doc(linkID)

	// Check if the link already exists
	doc, err := tx.Get(linkRef)
	if err == nil && doc.Exists() {
		// Link already exists, skip creation
		return nil
	}

	// Create the link
	link := JobDetailCriteriaLink{
		CriteriaID: criteriaID,
		JobID:      jobID,
		CreatedAt:  time.Now(),
	}

	return tx.Create(linkRef, &link)
}

// ListJobDetails retrieves all job details with optional limit and offset
func ListJobDetails(ctx context.Context, fs *FirestoreClient, limit int, offset int) ([]map[string]any, error) {
	query := fs.Collection("job_details").OrderBy("CreatedAt", firestore.Desc)
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}

	iter := query.Documents(ctx)
	var details []map[string]any

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate job details: %w", err)
		}

		details = append(details, doc.Data())
	}

	return details, nil
}

// DeleteJobDetailsByPrefix deletes all job details with IDs starting with the given prefix
func DeleteJobDetailsByPrefix(ctx context.Context, fs *FirestoreClient, prefix string) (int, error) {
	if prefix == "" {
		return 0, fmt.Errorf("prefix is required")
	}

	// Query for documents with IDs starting with the prefix
	iter := fs.Collection("job_details").Documents(ctx)
	deleted := 0

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return deleted, fmt.Errorf("failed to iterate job details: %w", err)
		}

		// Check if document ID starts with prefix
		if len(doc.Ref.ID) >= len(prefix) && doc.Ref.ID[:len(prefix)] == prefix {
			if _, err := doc.Ref.Delete(ctx); err != nil {
				return deleted, fmt.Errorf("failed to delete job detail %s: %w", doc.Ref.ID, err)
			}
			deleted++
		}
	}

	return deleted, nil
}

// GetJobDetailsByIDs retrieves multiple job details by their IDs
func GetJobDetailsByIDs(ctx context.Context, fs *FirestoreClient, ids []string) ([]*JobDetail, error) {
	if len(ids) == 0 {
		return []*JobDetail{}, nil
	}

	// First check cache
	var result []*JobDetail
	var missingIDs []string

	for _, id := range ids {
		if jobDetail, ok := fs.jobDetailCache.Get(id); ok {
			result = append(result, jobDetail)
		} else {
			missingIDs = append(missingIDs, id)
		}
	}

	// If all IDs were found in cache, return early
	if len(missingIDs) == 0 {
		return result, nil
	}

	// Create a batch of document references to get
	refs := make([]*firestore.DocumentRef, len(missingIDs))
	for i, id := range missingIDs {
		refs[i] = fs.Collection(JobDetailsCollection).Doc(id)
	}

	// Execute the batch get
	docs, err := fs.GetAll(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("failed to get job details: %w", err)
	}

	// Process results
	for i, doc := range docs {
		if doc.Exists() {
			var jobDetail JobDetail
			if err := doc.DataTo(&jobDetail); err != nil {
				return nil, fmt.Errorf("failed to parse job detail %s: %w", missingIDs[i], err)
			}

			// Add to cache
			fs.jobDetailCache.SetWithTTL(jobDetail.ID, &jobDetail, 1, 1*time.Hour)

			// Add to result
			result = append(result, &jobDetail)
		}
	}

	return result, nil
}
