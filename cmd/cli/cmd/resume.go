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
	Short: "Resume a user's resume",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}

var createResumeCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a resume for a user",
	Args:  cobra.ExactArgs(2),
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
	Use:   "annotate",
	Short: "Annotate a resume",
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
	Use:   "list",
	Short: "List resumes for a user",
	Args:  cobra.ExactArgs(1),
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
	Use:   "delete",
	Short: "Delete a resume from a user",
	RunE: func(cmd *cobra.Command, args []string) error {
		return nil
	},
}
