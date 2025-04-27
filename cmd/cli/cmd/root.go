package cmd

import (
	"github.com/spf13/cobra"
)

var (
	jsonOutput bool
)

var RootCmd = &cobra.Command{
	Use:   "cli",
	Short: "Talent Rodeo CLI tool",
	Long:  `A command line interface for managing Talent Rodeo operations`,
}

func init() {
	RootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}
