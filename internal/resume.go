package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal/funcs"
	"talent-rodeo/internal/utils"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	resumeCollection               = "resumes"
	resumeAnnotationPromptTemplate = "prompts/resume-annotation.001.txt"
)

type resumeAnalysis struct {
	Analysis    string
	IsResume    bool
	YAMLSummary *string
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

	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateResume creates a resume for a user in Firestore (using subcollection).
func CreateResume(ctx context.Context, fs *firestore.Client, user User, resume *Resume) error {
	if resume.RawText == "" {
		return fmt.Errorf("resume text is required")
	}

	if user.Name == "" {
		return fmt.Errorf("user name is required")
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

func ListResumes(ctx context.Context, fs *firestore.Client, userID string) ([]*Resume, error) {
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

// annotateResumeTestMode simulates resume annotation in test mode
func annotateResumeTestMode(logger *slog.Logger, user *User, resume *Resume) error {
	// Simulate processing delay
	time.Sleep(2 * time.Second)

	// Load test annotation data
	testFile := "testdata/sample-resumes/chris-cowherd.resume.annotated.json"
	data, err := os.ReadFile(testFile)
	if err != nil {
		return fmt.Errorf("failed to read test annotation data: %w", err)
	}

	// Parse the test data into resumeAnalysis struct
	var result resumeAnalysis
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("failed to unmarshal test data: %w", err)
	}

	// Update resume fields
	resume.IsValid = result.IsResume
	resume.Annotated = result.YAMLSummary
	resume.Analysis = &result.Analysis
	resume.UpdatedAt = time.Now()

	logger.Debug("using test annotation data",
		"file", testFile,
		"user_id", user.ID,
		"resume_id", resume.ID)

	return nil
}

// AnnotateResume uses an LLM to annotate the resume text and output it as YAML.
func AnnotateResume(ctx context.Context, fs *firestore.Client, logger *slog.Logger, user *User, resume *Resume) error {
	if os.Getenv("TESTING_MODE") == "true" {
		logger.Debug("annotateResume: using test mode")
		if err := annotateResumeTestMode(logger, user, resume); err != nil {
			logger.Error("annotateResume: test mode failed", "error", err)
			return fmt.Errorf("test mode annotation failed: %w", err)
		}

		// Update resume in Firestore and handle user state
		if err := UpdateResume(ctx, fs, *resume); err != nil {
			logger.Error("failed to update resume in Firestore", "error", err)
			return fmt.Errorf("failed to update resume in Firestore: %w", err)
		}

		if !resume.IsValid {
			errorMsg := "Uploaded document is not a valid resume"
			if err := UpdateUserStateWithError(ctx, fs, user, errorMsg); err != nil {
				logger.Error("failed to update user state with error", "error", err)
			}
		} else {
			if err := UpdateUserStatus(ctx, fs, user, UserStatusResumeUploaded); err != nil {
				logger.Error("failed to update user state to ResumeUploaded", "error", err)
			}
		}

		return nil
	}

	prompt, err := utils.CreateTemplate(resumeAnnotationPromptTemplate)
	if err != nil {
		return fmt.Errorf("convertResume: failed to create template: %w", err)
	}

	messages := []minds.Message{
		{
			Role:    minds.RoleDeveloper,
			Content: prompt.RawContent,
		},
		{
			Role:    minds.RoleUser,
			Content: resume.RawText,
		},
	}

	schema, err := minds.NewResponseSchema("resume_analysis", "Analysis of the resume", resumeAnalysis{})
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}
	req := minds.NewRequest(messages, minds.WithResponseSchema(*schema))

	llm, err := openai.NewProvider(openai.WithModel("gpt-4o"))
	if err != nil {
		return fmt.Errorf("failed to generate analysis: %w", err)
	}

	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to generate analysis: %w", err)
	}

	respJSON := resp.String()

	var result resumeAnalysis
	err = json.Unmarshal([]byte(respJSON), &result)
	if err != nil {
		return fmt.Errorf("failed to unmarshal analysis: %w", err)
	}

	resume.IsValid = result.IsResume
	resume.Annotated = result.YAMLSummary
	resume.Analysis = &result.Analysis

	err = UpdateResume(ctx, fs, *resume)
	if err != nil {
		return fmt.Errorf("failed to update resume in Firestore: %w", err)
	}

	if !resume.IsValid {
		errorMsg := "Uploaded document is not a valid resume"
		err = UpdateUserStateWithError(ctx, fs, user, errorMsg)
		if err != nil {
			logger.Error("failed to update user state with error", "error", err)
		}
	} else {
		// If the resume is valid, update the user state to ResumeUploaded
		err = UpdateUserStatus(ctx, fs, user, UserStatusResumeUploaded)
		if err != nil {
			logger.Error("failed to update user state to ResumeUploaded", "error", err)
		}
	}

	return nil
}

func UpdateResume(ctx context.Context, fs *firestore.Client, resume Resume) error {
	resume.UpdatedAt = time.Now()
	_, err := fs.Collection(usersCollection).Doc(resume.UserID).Collection(resumeCollection).Doc(resume.ID).Set(ctx, resume)
	if err != nil {
		return fmt.Errorf("failed to update resume: %w", err)
	}
	return nil
}

func GetResumes(ctx context.Context, fs *firestore.Client, userID string) ([]*Resume, error) {
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

func GetLatestResume(ctx context.Context, fs *firestore.Client, userID string) (*Resume, bool, error) {
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

func DeleteResume(ctx context.Context, fs *firestore.Client, userID string, resumeID string) error {
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
