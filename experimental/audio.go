package main

import (
	"encoding/binary"
	"math"
)

// ResampleAudio resamples audio data from original rate to target rate
func ResampleAudio(audio []byte, originalRate, targetRate int) ([]byte, error) {
	if originalRate == targetRate {
		return audio, nil
	}

	// Convert bytes to float64 samples
	samples := make([]float64, len(audio)/2)
	for i := 0; i < len(audio); i += 2 {
		samples[i/2] = float64(int16(binary.LittleEndian.Uint16(audio[i:])))
	}

	// Calculate resampling ratio
	ratio := float64(targetRate) / float64(originalRate)
	newLen := int(float64(len(samples)) * ratio)
	resampled := make([]float64, newLen)

	// Linear interpolation
	for i := range resampled {
		position := float64(i) / ratio
		index := int(position)
		if index >= len(samples)-1 {
			resampled[i] = samples[len(samples)-1]
			continue
		}
		fraction := position - float64(index)
		resampled[i] = samples[index]*(1-fraction) + samples[index+1]*fraction
	}

	// Convert back to bytes
	result := make([]byte, len(resampled)*2)
	for i, sample := range resampled {
		binary.LittleEndian.PutUint16(result[i*2:], uint16(math.Round(sample)))
	}

	return result, nil
}

// NormalizeValue normalizes a value between min and max to range [0,1]
func NormalizeValue(value, minValue, maxValue float64) float64 {
	normalized := (value - minValue) / (maxValue - minValue)
	if normalized < 0 {
		return 0
	}
	if normalized > 1 {
		return 1
	}
	return normalized
}

// CalculateAudioVolume calculates the RMS volume of audio data
func CalculateAudioVolume(audio []byte, sampleRate int) float64 {
	if len(audio) == 0 {
		return 0
	}

	var sum float64
	samples := len(audio) / 2

	for i := 0; i < len(audio); i += 2 {
		sample := float64(int16(binary.LittleEndian.Uint16(audio[i:])))
		sum += sample * sample
	}

	rms := math.Sqrt(sum / float64(samples))
	// Convert to dB scale (similar to loudness range in original)
	db := 20 * math.Log10(rms/32768.0)

	return NormalizeValue(db, -20, 80)
}

// ExpSmoothing performs exponential smoothing
func ExpSmoothing(value, prevValue, factor float64) float64 {
	return prevValue + factor*(value-prevValue)
}

// ULawToPCM converts μ-law encoded audio to PCM
func ULawToPCM(ulawBytes []byte, inSampleRate, outSampleRate int) ([]byte, error) {
	pcmBytes := make([]byte, len(ulawBytes)*2)
	for i, b := range ulawBytes {
		pcm := ulaw2lin(b)
		binary.LittleEndian.PutUint16(pcmBytes[i*2:], uint16(pcm))
	}

	return ResampleAudio(pcmBytes, inSampleRate, outSampleRate)
}

// PCMToULaw converts PCM audio to μ-law encoding
func PCMToULaw(pcmBytes []byte, inSampleRate, outSampleRate int) ([]byte, error) {
	resampled, err := ResampleAudio(pcmBytes, inSampleRate, outSampleRate)
	if err != nil {
		return nil, err
	}

	ulawBytes := make([]byte, len(resampled)/2)
	for i := 0; i < len(resampled); i += 2 {
		pcm := int16(binary.LittleEndian.Uint16(resampled[i:]))
		ulawBytes[i/2] = lin2ulaw(pcm)
	}

	return ulawBytes, nil
}

// Helper functions for μ-law conversion
func ulaw2lin(ulawByte byte) int16 {
	ulawByte = ^ulawByte
	sign := (ulawByte & 0x80) >> 7
	exponent := (ulawByte & 0x70) >> 4
	mantissa := ulawByte & 0x0F

	magnitude := ((int16(mantissa) << 3) + 0x84) << uint(exponent)
	if sign == 0 {
		return int16(magnitude)
	}
	return -int16(magnitude)
}

func lin2ulaw(pcm int16) byte {
	const BIAS = 0x84

	var sign int16 = 0x80
	if pcm < 0 {
		pcm = -pcm
		sign = 0
	}

	if pcm > 32635 {
		pcm = 32635
	}

	pcm += BIAS

	var exponent byte = 7
	for i := uint(0); i < 7; i++ {
		if pcm < 256 {
			exponent = byte(i)
			break
		}
		pcm >>= 1
	}

	mantissa := byte((pcm >> 3) & 0x0F)
	ulawByte := ^(byte(sign) | (exponent << 4) | mantissa)

	return ulawByte
}
