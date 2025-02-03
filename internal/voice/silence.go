package voice

import (
	"context"
	"time"
)

// SilenceDetector handles audio analysis for silence detection
type SilenceDetector struct {
	// RMS threshold below which we consider the audio to be silence
	threshold    float64
	silenceStart time.Time
	onSilence    func(duration time.Duration)

	// Sample rate of the audio (8000Hz for µ-law)
	sampleRate int
}

func NewSilenceDetector(threshold float64, sampleRate int) *SilenceDetector {
	return &SilenceDetector{
		threshold:  threshold, // sample must be louder than this to be considered non-silent
		sampleRate: sampleRate,
	}
}

func (a *SilenceDetector) Process(ctx context.Context, chunk AudioChunk) (AudioChunk, error) {
	nonSilentSamples, _ := a.GetAnalysis(chunk.Data)

	if nonSilentSamples == 0 {
		if a.silenceStart.IsZero() {
			a.silenceStart = time.Now()
		}
	} else {
		if !a.silenceStart.IsZero() {
			duration := time.Since(a.silenceStart)
			if duration > 0 {
				a.onSilence(duration)
			}
			a.silenceStart = time.Time{}
		}
	}

	return chunk, nil
}

func (a *SilenceDetector) Duration(ulawData []byte) time.Duration {
	if len(ulawData) == 0 {
		return 0
	}

	nonSilentSamples, _ := a.GetAnalysis(ulawData)

	duration := time.Duration(nonSilentSamples) * time.Second / time.Duration(a.sampleRate)

	return duration
}

func (a *SilenceDetector) GetAnalysis(ulawData []byte) (nonSilentSamples int, maxAmplitude float64) {
	if len(ulawData) == 0 {
		return
	}

	for _, ulawByte := range ulawData {
		pcm := float64(a.ulawToLinear(ulawByte)) / 32768.0
		if pcm < 0 {
			pcm = -pcm
		}
		if pcm > maxAmplitude {
			maxAmplitude = pcm
		}
		if pcm > a.threshold {
			nonSilentSamples++
		}
	}
	return
}

// ulawToLinear converts a µ-law sample to 16-bit linear PCM
func (a *SilenceDetector) ulawToLinear(ulawByte byte) int16 {
	// Flip all bits
	ulawByte = ^ulawByte

	// Extract sign bit
	sign := int16(1)
	if (ulawByte & 0x80) != 0 {
		sign = -1
	}

	// Extract and adjust magnitude
	position := (ulawByte >> 4) & 0x07
	magnitude := int16(((int16(ulawByte)&0x0F)<<3)+0x84) << position

	return sign * magnitude
}
