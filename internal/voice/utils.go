package voice

import (
	"time"
)

// InterruptionDetector implements AudioProcessor to detect when one party
// interrupts the other by monitoring both input and output streams.
type InterruptionDetector struct {
	inputBuffer  []AudioChunk
	outputBuffer []AudioChunk
	onInterrupt  func(who string, when time.Time)
}
