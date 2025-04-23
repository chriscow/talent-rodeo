package cmd

import (
	"github.com/spf13/cobra"
)

var RootCmd = &cobra.Command{
	Use:   "trcli",
	Short: "Talent Rodeo CLI tool",
	Long:  `A command line interface for managing Talent Rodeo operations`,
}
