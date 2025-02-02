package conversations

import (
	"github.com/chriscow/minds"
)

type roleReverserMiddleware struct{}

// ReverseRoles creates middleware that reverses the roles of messages
func ReverseRoles() minds.Middleware {
	return &roleReverserMiddleware{}
}

func (m *roleReverserMiddleware) Wrap(next minds.ThreadHandler) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, _ minds.ThreadHandler) (minds.ThreadContext, error) {
		tc, err := next.HandleThread(tc, nil)
		if err != nil {
			return tc, err
		}

		messages := tc.Messages()
		for i := 0; i < len(messages); i++ {
			switch messages[i].Role {
			case minds.RoleUser:
				messages[i].Role = minds.RoleAssistant
			case minds.RoleAssistant:
				messages[i].Role = minds.RoleUser
			}

			if messages[i].Content == "" {
				// this was likely a tool call - and the call ended.  skip this message
				continue
			}
		}

		tc = tc.WithMessages(messages...)

		return tc, nil
	})
}
