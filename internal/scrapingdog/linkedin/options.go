package linkedin

type JobSearchOptions struct {
	// This is an optional parameter through which you can pageLimit the number of
	// pages of job listings to return
	pageLimit int

	// This is an optional parameter through which you can sort jobs on the basis of
	// their posting dates.

	// Possible Values - day, week or month

	// `day` will provide jobs that were posted in the last 24 hours.
	// `week` will provide for the last 7 days and
	// `month` will provide for the last 30 days.
	sortBy string

	// 	This parameter will help you extract all the jobs according to the type of
	// 	job offered by the company.
	//
	// Possible Values - temporary, contract, volunteer, full_time  part_time.
	jobType string

	// This parameter will help you filter out the jobs based on the experience
	// required to apply for the job.
	//
	// Possible Values - internship, entry_level, associate, mid_senior_level
	// director.
	expLevel string

	// This parameter will help you filter out the jobs based on their working
	// model.
	//
	// Possible Values - at_work, remote,   hybrid.
	workType string

	// This parameter requires company LinkedIn ID and will help you get jobs
	// for that particular company.
	filterByCompany string
}

type JobSearchOption func(*JobSearchOptions)

func WithPageLimit(pageLimit int) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.pageLimit = pageLimit
	}
}

func WithSortBy(sortBy string) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.sortBy = sortBy
	}
}

func WithJobType(jobType string) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.jobType = jobType
	}
}

func WithExpLevel(expLevel string) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.expLevel = expLevel
	}
}

func WithWorkType(workType string) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.workType = workType
	}
}

func WithCompanyFilter(company string) JobSearchOption {
	return func(o *JobSearchOptions) {
		o.filterByCompany = company
	}
}
