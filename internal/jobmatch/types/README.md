# Types Package

The `types` package defines the core domain types and interfaces used throughout the job matching system. It provides the foundational data structures that represent jobs, candidates, and provider interactions.

## Key Components

### Provider Interface
```go
type JobProvider interface {
    SearchJobs(ctx context.Context, criteria SearchCriteria) ([]Job, error)
    GetJobDetails(ctx context.Context, jobID string) (*JobDetail, error)
    Name() string
    Validate(ctx context.Context) error
}
```

The `JobProvider` interface defines the contract that all job listing providers must implement. It's designed to be provider-agnostic while maintaining consistency in interactions.

### Core Types

#### Job
Represents a basic job listing with essential information:
- Basic job details (ID, title, company, etc.)
- Location and workplace type
- Industry and company information
- Provider-specific metadata

#### JobDetail
Extended job information including:
- Full job description
- Requirements and qualifications
- Benefits and salary information
- Company details
- Employment specifics

#### Candidate
Represents a job seeker with:
- Professional profile
- Skills and experience
- Location preferences
- Work preferences
- Education and work history

#### SearchCriteria
Defines job search parameters:
- Keywords and skills
- Location preferences
- Experience requirements
- Industry preferences
- Salary expectations

## Usage

The types in this package are used throughout the job matching system:

```go
// Creating search criteria
criteria := types.SearchCriteria{
    Keywords: []string{"golang", "kubernetes"},
    Location: "Remote",
    JobType:  []string{"full-time"},
}

// Implementing a provider
type MyProvider struct{}

func (p *MyProvider) SearchJobs(ctx context.Context, criteria SearchCriteria) ([]Job, error) {
    // Implementation
}
```

## Best Practices

1. Keep types immutable where possible
2. Use explicit field names in struct literals
3. Validate struct fields on creation
4. Use pointer receivers for methods that modify state
5. Document all exported types and fields

## Future Considerations

1. Phase 5 Enhancements:
   - Async operation support
   - Distributed processing fields
   - Enhanced validation
   - Performance optimizations

2. Planned Additions:
   - Enhanced company information
   - Detailed benefits structure
   - Equity compensation
   - Team structure data
   - Culture indicators

## Testing

The package includes:
- Type validation tests
- Interface compliance tests
- Serialization tests
- Field validation tests

## Dependencies

The package has minimal dependencies to maintain portability:
- Standard library only for core types
- Time handling for temporal fields
- Context for cancellation support 