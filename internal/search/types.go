package search

import (
	"context"
	"talent-rodeo/internal"
)

// SortByPeriod defines the possible values for the SortBy field
type SortByPeriod string

// WorkType defines the possible values for the WorkType field
type WorkType string

// JobType defines the possible values for the JobType field
type JobType string

// ExperienceLevel defines the possible values for the Experience field
type ExperienceLevel string

// Constants for message routing
const (
	TopicSearchInput  = "rodeo.input"
	TopicSearchResult = "rodeo.result"

	SortByDay   SortByPeriod = "day"   // Last 24 hours
	SortByWeek  SortByPeriod = "week"  // Last 7 days
	SortByMonth SortByPeriod = "month" // Last 30 days

	WorkTypeOffice WorkType = "at_work"
	WorkTypeRemote WorkType = "remote"
	WorkTypeHybrid WorkType = "hybrid"

	JobTypeFullTime  JobType = "full_time"
	JobTypePartTime  JobType = "part_time"
	JobTypeContract  JobType = "contract"
	JobTypeTemporary JobType = "temporary"
	JobTypeVolunteer JobType = "volunteer"

	ExperienceInternship ExperienceLevel = "internship"
	ExperienceEntryLevel ExperienceLevel = "entry_level"
	ExperienceAssociate  ExperienceLevel = "associate"
	ExperienceMidSenior  ExperienceLevel = "mid_senior_level"
	ExperienceDirector   ExperienceLevel = "director"
)

// JobEventHandler defines the interface for handling job-related events
type JobEventHandler interface {
	OnSearchStarted(task internal.SearchTask)
	OnJobListing(task internal.SearchTask, listing internal.JobListing)
	OnJobDetail(task internal.SearchTask, details internal.JobDetail)
	OnSearchCompleted(task internal.SearchTask, totalJobs int, errors []error)
}

type JobQueue interface {
	QueueSearch(ctx context.Context, search internal.SearchTask) error
	GetQueueDepth(ctx context.Context) (int, error)
}

// JobProvider interface defines methods that must be implemented by all job providers
type JobProvider interface {
	Name() string
	Validate(ctx context.Context) error
	SearchJobListings(ctx context.Context, criteria internal.SearchCriteria) ([]internal.JobListing, error)
	GetJobDetails(ctx context.Context, jobID string) (*internal.JobDetail, error)
}

type JobCache interface {
	GetJobListings(ctx context.Context, key string) ([]internal.JobListing, bool)
	GetJobDetail(ctx context.Context, key string) (*internal.JobDetail, bool)
}
