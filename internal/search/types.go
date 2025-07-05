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

type JobCache interface {
	GetJobListings(ctx context.Context, key string) ([]internal.JobListing, bool)
	GetJobDetail(ctx context.Context, key string) (*internal.JobDetail, bool)
}
