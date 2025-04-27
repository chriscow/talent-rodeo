package try

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

// TryCmd represents the base command when called without any subcommands
var TryCmd = &cobra.Command{
	Use: "try",
	// Uncomment the following line if your bare application
	// has an action associated with it:
	// Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := godotenv.Overload(); err != nil {
		log.Fatal(err)
	}

	err := TryCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
