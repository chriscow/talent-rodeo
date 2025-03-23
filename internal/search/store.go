package search

import (
	"context"
)

type JobStoreInterface interface {
	GetCachedJobListings(criteria Criteria) ([]JobListing, error)
	SaveJobListings(ctx context.Context, criteria Criteria, jobs []JobListing) error
}
