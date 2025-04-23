package main

import (
	"fmt"
	"os"
	"talent-rodeo/cmd/trcli/cmd"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Overload(); err != nil {
		fmt.Printf("Error loading .env file: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
