package main

import (
	"log"
	"net/http"
	"talent-rodeo/cmd/try"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Overload(); err != nil {
		panic(err)
	}

	/*
		1. Run your program and then open your browser and navigate to http://localhost:6060/debug/pprof/ to see various profiling data.

		2. Generate a CPU profile:

			go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

		3. Analyze the profile:

			go tool pprof cpu.prof
	*/
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	try.Execute()
}
