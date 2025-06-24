package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"talent-rodeo/assets"
	"talent-rodeo/internal/funcs"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/tools"
	"github.com/sashabaranov/go-openai"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	resumeCollection             = "resumes"
	resumeAnalysisPrompt         = "prompts/resume.legit-analysis.v001.txt"
	resumeAnalysisExamples       = "prompts/resume.legit-analysis.examples.v001.txt"
	resumeSearchAnalysisTemplate = "prompts/resume.search-analysis.v001.txt"
)

type annotatedResume struct {
	Analysis string
	IsResume bool
	Summary  *string
}

type ErrResumeExists struct {
	ID string
}

func (e ErrResumeExists) Error() string {
	return fmt.Sprintf("resume already exists: %s", e.ID)
}

type Resume struct {
	ID     string
	UserID string

	// JobID is the ID of the job that the resume is customized for.
	JobID *string

	RawText string

	// Annotated is the LLM annotated resume text.
	Annotated *string

	// Validated is true if the resume has been validated.
	IsValid bool

	// Analysis is the LLM analysis of the resume.
	Analysis *string

	// This is the LLM's determination of the search terms
	SearchAnalysis *string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateResume creates a resume for a user in Firestore (using subcollection).
func CreateResume(ctx context.Context, fs *FirestoreClient, user User, resume *Resume) error {
	if fs == nil {
		return fmt.Errorf("Firestore client is nil")
	}

	if resume.RawText == "" {
		return fmt.Errorf("resume text is required")
	}

	if resume.ID == "" {
		if resume.JobID != nil {
			resume.ID = funcs.Slugify(fmt.Sprintf("resume-for-jobid-%s-%d", funcs.Slugify(*resume.JobID), time.Now().UnixNano()))
		} else {
			resume.ID = fmt.Sprintf("%d", time.Now().UnixNano())
		}
	}

	resume.UserID = user.ID
	resume.CreatedAt = time.Now()
	resume.UpdatedAt = time.Now()

	// 1. Reference the user document:
	userDocRef := fs.Collection(usersCollection).Doc(user.ID)

	// 2. Reference the resumes subcollection within the user document:
	resumesCollectionRef := userDocRef.Collection(resumeCollection)

	// 3. Add the resume document to the resumes subcollection:
	_, err := resumesCollectionRef.Doc(resume.ID).Set(ctx, resume) // Use Set to create or overwrite
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return ErrResumeExists{ID: resume.ID}
		}
		return fmt.Errorf("failed to create resume in subcollection: %w", err)
	}

	return nil
}

func GetResume(ctx context.Context, fs *FirestoreClient, userID string, resumeID string) (*Resume, bool, error) {
	if fs == nil {
		return nil, false, fmt.Errorf("Firestore client is nil")
	}

	if userID == "" {
		return nil, false, fmt.Errorf("user ID for Firestore is required")
	}
	if resumeID == "" {
		return nil, false, fmt.Errorf("resume ID for Firestore is required")
	}

	doc, err := fs.Collection(usersCollection).Doc(userID).Collection(resumeCollection).Doc(resumeID).Get(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get resume: %w", err)
	}

	var resume Resume
	if err := doc.DataTo(&resume); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal resume data: %w", err)
	}

	return &resume, true, nil
}

func ListResumes(ctx context.Context, fs *FirestoreClient, userID string) ([]*Resume, error) {
	if fs == nil {
		return nil, fmt.Errorf("Firestore client is nil")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID for Firestore is required")
	}

	iter := fs.Collection(usersCollection).Doc(userID).Collection(resumeCollection).Documents(ctx)

	var resumes []*Resume
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate resumes: %w", err)
		}

		var resume Resume
		if err := doc.DataTo(&resume); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resume data: %w", err)
		}
		resumes = append(resumes, &resume)
	}

	return resumes, nil
}

// AnnotateResume uses an LLM to annotate the resume text and output it as YAML.
func AnnotateResume(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, user *User, resume *Resume) error {
	if fs == nil {
		return fmt.Errorf("Firestore client is nil")
	}

	if resume == nil {
		return fmt.Errorf("resume is nil")
	}

	if user == nil {
		return fmt.Errorf("user is nil")
	}

	var analysis *annotatedResume
	var searchAnalysis string
	var err error
	if os.Getenv("TESTING_MODE") == "true" {
		logger.Debug("annotateResume: using test mode")
		analysis, err = getResumeAnalysisTestMode(ctx, *resume)
		if err != nil {
			logger.Error("annotateResume: test mode failed", "error", err)
			return fmt.Errorf("test mode annotation failed: %w", err)
		}

		searchAnalysis, err = getResumeSearchAnalysisTestMode(ctx, *resume)
		if err != nil {
			logger.Error("annotateResume: test mode search analysis failed", "error", err)
			return fmt.Errorf("test mode search analysis failed: %w", err)
		}
	} else {
		analysis, err = getAnnotatedResume(ctx, fs, logger, user.ID, *resume)
		if err != nil {
			return fmt.Errorf("failed to get resume analysis: %w", err)
		}

		searchAnalysis, err = getResumeSearchAnalysis(ctx, fs, logger, user.ID, *resume)
		if err != nil {
			return fmt.Errorf("failed to get resume search analysis: %w", err)
		}
	}

	resume.IsValid = analysis.IsResume
	resume.Annotated = analysis.Summary
	resume.Analysis = &analysis.Analysis
	resume.UpdatedAt = time.Now()
	resume.SearchAnalysis = &searchAnalysis

	if err := UpdateResume(ctx, fs, *resume); err != nil {
		return fmt.Errorf("failed to update resume in Firestore: %w", err)
	}

	if !resume.IsValid {
		errorMsg := "Uploaded document is not a valid resume"
		if resume.Analysis != nil && (strings.Contains(*resume.Analysis, "news article") || strings.Contains(*resume.Analysis, "jailbreak")) {
			errorMsg = "The document you uploaded doesn't appear to be a valid resume. Please upload a proper resume document."
		}
		if err := UpdateUserStateWithError(ctx, fs, user, errorMsg); err != nil {
			logger.Error("failed to update user state with error", "error", err)
		}
	} else {
		// If the resume is valid, update the user state to ResumeUploaded
		if err := UpdateUserStatus(ctx, fs, user, UserStatusResumeUploaded); err != nil {
			logger.Error("failed to update user state to ResumeUploaded", "error", err)
		}
	}

	return nil
}

func UpdateResume(ctx context.Context, fs *FirestoreClient, resume Resume) error {
	if fs == nil {
		return fmt.Errorf("Firestore client is nil")
	}

	resume.UpdatedAt = time.Now()
	_, err := fs.Collection(usersCollection).Doc(resume.UserID).Collection(resumeCollection).Doc(resume.ID).Set(ctx, resume)
	if err != nil {
		return fmt.Errorf("failed to update resume: %w", err)
	}
	return nil
}

func GetResumes(ctx context.Context, fs *FirestoreClient, userID string) ([]*Resume, error) {
	if fs == nil {
		return nil, fmt.Errorf("Firestore client is nil")
	}

	if userID == "" {
		return nil, fmt.Errorf("user ID for Firestore is required")
	}

	iter := fs.Collection(usersCollection).Doc(userID).Collection(resumeCollection).Documents(ctx)

	var resumes []*Resume
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate resumes: %w", err)
		}

		var resume Resume
		if err := doc.DataTo(&resume); err != nil {
			return nil, fmt.Errorf("failed to unmarshal resume data: %w", err)
		}
		resumes = append(resumes, &resume)
	}

	return resumes, nil
}

func GetLatestResume(ctx context.Context, fs *FirestoreClient, userID string) (*Resume, bool, error) {
	if fs == nil {
		return nil, false, fmt.Errorf("Firestore client is nil")
	}

	if userID == "" {
		return nil, false, fmt.Errorf("user ID for Firestore is required")
	}

	iter := fs.Collection(usersCollection).Doc(userID).Collection(resumeCollection).OrderBy("CreatedAt", firestore.Desc).Limit(1).Documents(ctx)
	doc, err := iter.Next()
	if err != nil {
		if err == iterator.Done {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get latest resume: %w", err)
	}

	var resume Resume
	if err := doc.DataTo(&resume); err != nil {
		return nil, false, fmt.Errorf("failed to unmarshal resume data: %w", err)
	}

	return &resume, true, nil
}

func DeleteResume(ctx context.Context, fs *FirestoreClient, userID string, resumeID string) error {
	if fs == nil {
		return fmt.Errorf("Firestore client is nil")
	}

	if userID == "" {
		return fmt.Errorf("user ID for Firestore is required")
	}
	if resumeID == "" {
		return fmt.Errorf("resume ID for Firestore is required")
	}

	_, err := fs.Collection(usersCollection).Doc(userID).Collection(resumeCollection).Doc(resumeID).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete resume: %w", err)
	}
	return nil
}

func getAnnotatedResume(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, userID string, resume Resume) (*annotatedResume, error) {
	sysPrompt, err := assets.EmbeddedFiles.ReadFile(resumeAnalysisPrompt)
	if err != nil {
		return nil, fmt.Errorf("convertResume: failed to read resume annotation prompt template: %w", err)
	}

	examples, err := assets.EmbeddedFiles.ReadFile(resumeAnalysisExamples)
	if err != nil {
		return nil, fmt.Errorf("convertResume: failed to read resume analysis examples: %w", err)
	}

	sysPrompt = append(sysPrompt, examples...)
	msgs := minds.Messages{
		{
			Role:    minds.RoleSystem,
			Content: string(sysPrompt),
		},
	}

	resp := openai.ChatCompletionResponse{}
	analysis, err := tools.Ask(ctx, resume.RawText, tools.WithMessages(msgs), tools.WithModel(tools.DeepSeekChat), tools.WithMaxTokens(4096), tools.WantCompletionResponse(&resp))
	if err != nil {
		return nil, fmt.Errorf("convertResume: failed to ask LLM: %w", err)
	}

	// Record LLM cost for resume analysis
	if err := RecordLLMCost(ctx, fs, &userID, tools.DeepSeekChat, resp, "resume_analysis", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for resume analysis", "error", err)
	}

	extract, err := extractResumeAnalysis(analysis)
	if err != nil {
		return extractResumeAnalysisLLM(ctx, fs, logger, userID, analysis)
	}

	type userName struct {
		FirstName string `description:"First name of the user"`
		LastName  string `description:"Last name of the user"`
		FullName  string `description:"Full name of the user"`
	}

	user, found, err := GetUser(ctx, fs, userID)
	if err != nil {
		return nil, fmt.Errorf("convertResume: failed to get user: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("convertResume: user not found: %s", userID)
	}

	if user.Name == "" {
		userName, err := tools.StructuredAsk[userName](ctx, "Extract the user's name from the resume text: "+resume.RawText, tools.WithModel(tools.GPT41Nano), tools.WithMaxTokens(4096))
		if err != nil {
			return nil, fmt.Errorf("convertResume: failed to extract user name: %w", err)
		}

		if err := UpdateUser(ctx, fs, userID, map[string]any{
			"Name":      userName.FullName,
			"FirstName": userName.FirstName,
			"LastName":  userName.LastName,
		}); err != nil {
			return nil, fmt.Errorf("convertResume: failed to update user with name: %w", err)
		}
	}

	return extract, nil
}

func extractResumeAnalysisLLM(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, userID string, analysis string) (*annotatedResume, error) {
	prompt := "Extract the resume analysis from the following text: " + analysis

	resp := openai.ChatCompletionResponse{}
	res, err := tools.StructuredAsk[annotatedResume](ctx, prompt, tools.WithModel(tools.GPT41Nano), tools.WithMaxTokens(4096), tools.WantCompletionResponse(&resp))
	if err != nil {
		return nil, fmt.Errorf("convertResume: failed to ask LLM: %w", err)
	}

	// Record LLM cost for resume analysis extraction
	if err := RecordLLMCost(ctx, fs, &userID, tools.GPT41Nano, resp, "resume_analysis_extraction", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for resume analysis extraction", "error", err)
	}

	return &res, nil
}

func extractResumeAnalysis(analysis string) (*annotatedResume, error) {
	var isValidResume string
	var resumeBreakdown string
	var summary string

	if strings.Contains(analysis, "<resume_analysis>") && strings.Contains(analysis, "</resume_analysis>") {
		// Extract the string between the <resume_analysis> tags
		// and remove the tags
		start := strings.Index(analysis, "<resume_analysis>")
		if start == -1 {
			return nil, fmt.Errorf("failed to extract resume analysis: <resume_analysis> tag not found")
		}
		end := strings.Index(analysis, "</resume_analysis>")
		if end == -1 {
			return nil, fmt.Errorf("failed to extract resume analysis: </resume_analysis> tag not found")
		}
		if end < start {
			return nil, fmt.Errorf("failed to extract resume analysis: </resume_analysis> tag is before <resume_analysis> tag")
		}
		analysis = analysis[start+len("<resume_analysis>") : end]

		if strings.Contains(analysis, "<is_valid_resume>") && strings.Contains(analysis, "</is_valid_resume>") {
			start = strings.Index(analysis, "<is_valid_resume>")
			if start == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: <is_valid_resume> tag not found")
			}
			end = strings.Index(analysis, "</is_valid_resume>")
			if end == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: </is_valid_resume> tag not found")
			}
			if end < start {
				return nil, fmt.Errorf("failed to extract resume analysis: </is_valid_resume> tag is before <is_valid_resume> tag")
			}
			isValidResume = analysis[start+len("<is_valid_resume>") : end]
		}

		if strings.Contains(analysis, "<resume_breakdown>") && strings.Contains(analysis, "</resume_breakdown>") {
			start = strings.Index(analysis, "<resume_breakdown>")
			if start == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: <resume_breakdown> tag not found")
			}
			end = strings.Index(analysis, "</resume_breakdown>")
			if end == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: </resume_breakdown> tag not found")
			}
			if end < start {
				return nil, fmt.Errorf("failed to extract resume analysis: </resume_breakdown> tag is before <resume_breakdown> tag")
			}
			resumeBreakdown = analysis[start+len("<resume_breakdown>") : end]
		}

		if strings.Contains(analysis, "<summary>") && strings.Contains(analysis, "</summary>") {
			start = strings.Index(analysis, "<summary>")
			if start == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: <summary> tag not found")
			}
			end = strings.Index(analysis, "</summary>")
			if end == -1 {
				return nil, fmt.Errorf("failed to extract resume analysis: </summary> tag not found")
			}
			summary = analysis[start+len("<summary>") : end]
		}

		return &annotatedResume{
			IsResume: isValidResume == "true",
			Analysis: resumeBreakdown,
			Summary:  &summary,
		}, nil
	}

	return nil, fmt.Errorf("failed to extract resume analysis: <resume_analysis> tag not found")
}

func getResumeSearchAnalysis(ctx context.Context, fs *FirestoreClient, logger *slog.Logger, userID string, resume Resume) (string, error) {
	tmpl, err := assets.EmbeddedFiles.ReadFile(resumeSearchAnalysisTemplate)
	if err != nil {
		return "", fmt.Errorf("convertResume: failed to read resume annotation prompt template: %w", err)
	}

	prompt, err := tools.GeneratePrompt(tmpl, map[string]any{
		"Resume": resume.RawText,
	})
	if err != nil {
		return "", fmt.Errorf("convertResume: failed to generate prompt: %w", err)
	}

	resp := openai.ChatCompletionResponse{}
	result, err := tools.Ask(ctx, prompt, tools.WithModel(tools.DeepSeekChat), tools.WantCompletionResponse(&resp))
	if err != nil {
		return "", err
	}

	// Record LLM cost for resume search analysis
	if err := RecordLLMCost(ctx, fs, &userID, tools.DeepSeekChat, resp, "resume_search_analysis", nil); err != nil {
		// Log error but don't fail the operation
		logger.Error("failed to record LLM cost for resume search analysis", "error", err)
	}
	return result, nil
}

// getResumeAnalysisTestMode simulates resume annotation in test mode
func getResumeAnalysisTestMode(_ context.Context, _ Resume) (*annotatedResume, error) {
	// Simulate processing delay
	time.Sleep(2 * time.Second)

	// Load test annotation data
	testFile := "testdata/sample-resumes/chris-cowherd.resume.annotated.json"
	data, err := os.ReadFile(testFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read test annotation data: %w", err)
	}

	// Parse the test data into resumeAnalysis struct
	var result annotatedResume
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	return &result, nil
}

func getResumeSearchAnalysisTestMode(_ context.Context, _ Resume) (string, error) {
	// Simulate processing delay
	time.Sleep(1 * time.Second)

	// Load test search analysis data
	testFile := "testdata/sample-resumes/chris-cowherd.resume.search-analysis.txt"
	data, err := os.ReadFile(testFile)
	if err != nil {
		return "", fmt.Errorf("failed to read test search analysis data: %w", err)
	}

	// Parse the test data into a string
	result := string(data)

	return result, nil
}
