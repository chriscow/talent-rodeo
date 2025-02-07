package handlers

const (
	// Topics for each stage of analysis
	TopicTranscriptInput       = "rodeo.transcript.input"
	TopicJobDescriptionInput   = "rodeo.jobdesc.input"
	TopicRequirementsExtracted = "rodeo.jobdesc.extracted.requirements"
	TopicCompensationExtracted = "rodeo.jobdesc.extracted.compensation"
	TopicWorkplaceExtracted    = "rodeo.jobdesc.extracted.workplace"
	TopicCultureExtracted      = "rodeo.jobdesc.extracted.culture"

	// I'm not going to send a completion message
	// TopicJobDescriptionAnalyzed = "rodeo.jobdesc.analyzed"

	// Stream names are commonly uppercased to visually differentiate them from subjects
	rodeoStream            = "RODEO"
	jobDescProcessingQueue = "RODEO-JOBDESC-QUEUE"
)

type TranscriptInput struct {
	Content string `json:"content" description:"Transcript text to analyze"`
}

// JobDescriptionInput represents the input for job description analysis
type JobDescriptionInput struct {
	Description string `json:"description" description:"Job description text to analyze"`
}
