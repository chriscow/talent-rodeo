package conversations

import (
	"database/sql"
	"time"

	"github.com/chriscow/minds"
)

type PersistenceConfig struct {
	DB             *sql.DB
	ConversationID string
	RunID          string
}

type persistenceMiddleware struct {
	cfg PersistenceConfig
}

// PersistMessage creates middleware that saves each message to the database
func PersistMessage(cfg PersistenceConfig) minds.Middleware {
	return &persistenceMiddleware{cfg: cfg}
}

func (m *persistenceMiddleware) Wrap(next minds.ThreadHandler) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, _ minds.ThreadHandler) (minds.ThreadContext, error) {
		// Get the last message which is the one we want to save
		messages := tc.Messages()
		if len(messages) == 0 {
			return next.HandleThread(tc, nil)
		}
		lastMsg := messages[len(messages)-1]

		// Save the message
		_, err := m.cfg.DB.Exec(`
			INSERT INTO messages (conversation_id, run_id, name, content, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			m.cfg.ConversationID,
			m.cfg.RunID,
			lastMsg.Name,
			lastMsg.Content,
			time.Now(),
		)
		if err != nil {
			return tc, err
		}

		// Continue the chain
		return next.HandleThread(tc, nil)
	})
}
