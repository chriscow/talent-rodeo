package search

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
