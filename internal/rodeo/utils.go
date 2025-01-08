package rodeo

import "fmt"

type defaultStatus struct {
}

func (l defaultStatus) SendStatus(sessionID, status, message string) {
	fmt.Printf("SessionID: %s, Status: %s, Message: %s\n", sessionID, status, message)
}
