package rodeo

import "fmt"

type StatusSender interface {
	SendStatus(sessionID, level, message string)
}

type defaultStatus struct {
}

func (l defaultStatus) SendStatus(sessionID, status, message string) {
	fmt.Printf("SessionID: %s, Status: %s, Message: %s\n", sessionID, status, message)
}
