package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"talent-rodeo/internal"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(resumeCmd)
	resumeCmd.AddCommand(createResumeCmd)
	resumeCmd.AddCommand(listResumesCmd)
	resumeCmd.AddCommand(deleteResumeCmd)
	resumeCmd.AddCommand(annotateResumeCmd)

	annotateResumeCmd.Flags().StringVarP(&userID, "user", "u", "", "User ID")
	annotateResumeCmd.Flags().StringVarP(&resumeID, "resume", "r", "", "Resume ID")
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Manage user resumes and AI analysis",
	Long: `Manage user resumes, upload resume content, and run AI analysis.

Resumes are core to job matching as they contain user qualifications,
experience, and skills. The system can analyze resumes with AI to extract
structured data and improve job matching accuracy.

This command group allows you to:
- Create resumes from text files  
- List resumes for users
- Run AI analysis on resume content
- Delete resume records`,
}

var createResumeCmd = &cobra.Command{
	Use:   "create <user-id> <resume-file>",
	Short: "Create a resume for a user from a text file",
	Long: `Create a resume record for a user from a text file.

Reads the resume content from a file and creates a resume record in the
system. The resume is automatically analyzed with AI to extract structured
information like skills, experience, and qualifications.

The file should contain the resume text in plain text format.

Examples:
  trcli resume create user123 resume.txt              # Create resume from file`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		userID := args[0]
		if _, err := uuid.Parse(userID); err != nil {
			return fmt.Errorf("invalid user ID: %w", err)
		}

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		data, err := os.ReadFile(args[1])
		if err != nil {
			return fmt.Errorf("failed to read resume file: %w", err)
		}

		resume := &internal.Resume{
			RawText: string(data),
		}

		err = internal.CreateResume(ctx, fs, *user, resume)
		if err != nil {
			return fmt.Errorf("failed to add resume: %w", err)
		}

		err = internal.AnnotateResume(ctx, fs, slog.Default(), user, resume)
		if err != nil {
			return fmt.Errorf("failed to annotate resume: %w", err)
		}

		fmt.Printf("Added resume: %s to user: %s\n", resume.ID, user.Name)
		return nil
	},
}

var annotateResumeCmd = &cobra.Command{
	Use:   "annotate --user <user-id> --resume <resume-id>",
	Short: "Run AI analysis on an existing resume",
	Long: `Run AI analysis on an existing resume to extract structured information.

This command uses AI to analyze resume content and extract structured data
like skills, experience levels, job titles, and qualifications. This
information is used to improve job matching accuracy.

Required flags:
  --user, -u: User ID who owns the resume
  --resume, -r: Resume ID to analyze

Examples:
  trcli resume annotate --user user123 --resume resume456`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		if userID == "" {
			return fmt.Errorf("user ID flag is required")
		}

		user, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		if resumeID == "" {
			return fmt.Errorf("resume ID flag is required")
		}

		resume, found, err := internal.GetResume(ctx, fs, userID, resumeID)
		if err != nil {
			return fmt.Errorf("failed to get resume: %w", err)
		}
		if !found {
			return fmt.Errorf("resume not found: %s", resumeID)
		}

		err = internal.AnnotateResume(ctx, fs, slog.Default(), user, resume)
		if err != nil {
			return fmt.Errorf("failed to annotate resume: %w", err)
		}

		fmt.Printf("Annotated resume: %s for user: %s\n", resume.ID, user.Name)
		return nil
	},
}

var listResumesCmd = &cobra.Command{
	Use:   "list <user-id>",
	Short: "List all resumes for a user",
	Long: `List all resume records for a specific user.

Shows resume IDs and metadata for all resumes associated with the user.
Useful for finding resume IDs for other operations or checking what
resumes a user has uploaded.

Examples:
  trcli resume list user123                           # List resume IDs
  trcli resume list user123 --json                    # Get full resume data`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		fs, err := internal.NewFirestoreClient(ctx)
		if err != nil {
			return fmt.Errorf("failed to create firestore client: %w", err)
		}
		defer fs.Close()

		userID := args[0]
		if _, err := uuid.Parse(userID); err != nil {
			return fmt.Errorf("invalid user ID: %w", err)
		}

		_, found, err := internal.GetUser(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}
		if !found {
			return fmt.Errorf("user not found: %s", userID)
		}

		resumes, err := internal.ListResumes(ctx, fs, userID)
		if err != nil {
			return fmt.Errorf("failed to list resumes: %w", err)
		}

		if len(resumes) == 0 {
			fmt.Printf("No resumes found for user: %s\n", userID)
			return nil
		}

		if jsonOutput {
			res, err := json.MarshalIndent(resumes, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal resumes: %w", err)
			}
			fmt.Println(string(res))
		} else {
			for _, resume := range resumes {
				fmt.Println(resume.ID)
			}
		}

		return nil
	},
}

var deleteResumeCmd = &cobra.Command{
	Use:   "delete <user-id> <resume-id>",
	Short: "Delete a resume record from a user",
	Long: `Delete a specific resume record from a user's profile.

WARNING: This permanently removes the resume data and cannot be undone.
The user will need to re-upload their resume if deleted.

Examples:
  trcli resume delete user123 resume456               # Delete specific resume`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
