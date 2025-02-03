package voice

import (
	"fmt"
	"sync"
	"time"
)

// AudioBuffer manages buffering and analysis of audio data
type AudioBuffer struct {
	data           []byte
	duration       time.Duration
	mu             sync.Mutex
	sd             *SilenceDetector
	lastAppendTime time.Time
}

const silenceThreshold = 0.1
const ulawSampleRate = 8000

func NewAudioBuffer() *AudioBuffer {
	return &AudioBuffer{
		data: make([]byte, 0),
		sd:   NewSilenceDetector(silenceThreshold, ulawSampleRate),
	}
}

// AddData adds new audio data to the buffer and returns the total duration of non-silent data buffered
func (b *AudioBuffer) AddData(data []byte) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.data = append(b.data, data...)

	now := time.Now()
	if len(b.data) > 0 && b.lastAppendTime.IsZero() {
		b.lastAppendTime = now
	}

	if len(b.data) > 0 && now.Sub(b.lastAppendTime) >= 5*time.Second {
		nonSilentSamples, maxAmplitude := b.sd.GetAnalysis(b.data)
		fmt.Printf("Audio analysis: samples=%d, nonSilent=%d, maxAmplitude=%f, threshold=%f\n",
			len(b.data), nonSilentSamples, maxAmplitude, silenceThreshold)
		b.lastAppendTime = now
	}

	duration := b.sd.Duration(b.data)
	return duration
}

// FlushData returns the buffered data and clears the buffer
func (b *AudioBuffer) FlushData() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	data := make([]byte, len(b.data))
	copy(data, b.data)
	b.data = make([]byte, 0)
	b.duration = 0
	return data
}
