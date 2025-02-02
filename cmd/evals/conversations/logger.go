package conversations

import (
	"fmt"
	"sync"

	"github.com/chriscow/minds"
	"github.com/fatih/color"
)

var (
	// Available colors for participants
	colors = []*color.Color{
		color.New(color.FgCyan),
		color.New(color.FgMagenta),
		color.New(color.FgYellow),
		color.New(color.FgGreen),
		color.New(color.FgBlue),
		color.New(color.FgRed),
		color.New(color.FgHiCyan),
		color.New(color.FgHiMagenta),
		color.New(color.FgHiYellow),
		color.New(color.FgHiGreen),
		color.New(color.FgHiBlue),
		color.New(color.FgHiRed),
	}
)

type conversationLogger struct {
	colorMap  map[string]*color.Color
	colorIdx  int
	colorLock sync.RWMutex
}

// ConversationLogger creates middleware that logs messages with colored participant names
func ConversationLogger() minds.Middleware {
	return &conversationLogger{
		colorMap: make(map[string]*color.Color),
	}
}

func (l *conversationLogger) getColor(name string) *color.Color {
	l.colorLock.RLock()
	c, exists := l.colorMap[name]
	l.colorLock.RUnlock()

	if exists {
		return c
	}

	l.colorLock.Lock()
	defer l.colorLock.Unlock()

	// Check again in case another goroutine assigned a color while we were waiting
	if c, exists = l.colorMap[name]; exists {
		return c
	}

	// Assign next color
	c = colors[l.colorIdx%len(colors)]
	l.colorMap[name] = c
	l.colorIdx++

	return c
}

func (l *conversationLogger) Wrap(next minds.ThreadHandler) minds.ThreadHandler {
	return minds.ThreadHandlerFunc(func(tc minds.ThreadContext, _ minds.ThreadHandler) (minds.ThreadContext, error) {
		// Get the last message which is the one we want to log
		messages := tc.Messages()
		if len(messages) > 0 {
			lastMsg := messages[len(messages)-1]
			if lastMsg.Name != "" {
				color := l.getColor(lastMsg.Name)
				fmt.Printf("%s %s\n",
					color.Sprintf("[%s]", lastMsg.Name),
					lastMsg.Content,
				)
			}
		}

		// Continue the chain
		return next.HandleThread(tc, nil)
	})
}
