package internal

import "time"

const (
	JobDetailFingerprintsCollection = "job_detail_fingerprints"
)

// JobDetailFingerprint links a description hash to a canonical job and all its duplicates.
// The document ID for this struct in Firestore is the SHA256 hash of the job description.
type JobDetailFingerprint struct {
	ID             string    // The description hash
	CanonicalJobID string    // The ID of the first JobDetail seen with this description.
	ProviderJobIDs []string  // A list of all provider-specific job IDs that share this description.
	CreatedAt      time.Time //
	UpdatedAt      time.Time //
}
