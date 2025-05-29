package main

import (
	"flag"
	"fmt"
	"os"
	"talent-rodeo/cmd/cli/cmd"

	"github.com/joho/godotenv"
)

func main() {
	envFile := flag.String("env", ".env", "Path to the .env file")
	flag.Parse()

	if err := godotenv.Overload(*envFile); err != nil {
		fmt.Printf("Error loading .env file: %s err: %v\n", envFile, err)
		os.Exit(1)
	}

	if err := cmd.RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
