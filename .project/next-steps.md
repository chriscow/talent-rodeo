1. When a VAPI call completes, we currently get an "end of call report"@webhooks.go 
2. We need a way to register a handler func with the @webhooks.go to let our @server.go get notificed of when the call ends.
3. The above handler needs to POST the call ID to a new job search service (new CLI tool in `/cmd`)
3. The job search service will call @call.go "GetCall" to get the full transcript ("CallResult.Messages")
4. The job search service will take all the messages and using "github.com/sashabaranov/go-openai" will prompt OpenAI to create a first person
1. call "GetCall" to get the transcript
3. The 