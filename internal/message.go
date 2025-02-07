package internal

import "time"

// Message represents a phone call message
type Message struct {
	ID        string    `json:"id" db:"id"`
	To        string    `json:"to" db:"to"`
	From      string    `json:"from" db:"from"`
	Body      string    `json:"body" db:"body"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}
