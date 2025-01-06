package main

import (
	"context"
	"encoding/base64"
	"sync"

	"github.com/gorilla/websocket"
)

type AudioInterface interface {
	Start(callback func([]byte))
	Stop()
	Output(audio []byte)
	Interrupt()
}

type TwilioAudioInterface struct {
	conn          *websocket.Conn
	inputCallback func([]byte)
	streamSid     string
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewTwilioAudioInterface(conn *websocket.Conn) *TwilioAudioInterface {
	ctx, cancel := context.WithCancel(context.Background())
	return &TwilioAudioInterface{
		conn:   conn,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (t *TwilioAudioInterface) Start(callback func([]byte)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputCallback = callback
}

func (t *TwilioAudioInterface) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inputCallback = nil
	t.streamSid = ""
	t.cancel()
}

func (t *TwilioAudioInterface) Output(audio []byte) {
	go func() {
		t.mu.Lock()
		streamSid := t.streamSid
		t.mu.Unlock()

		if streamSid == "" {
			return
		}

		audioPayload := base64.StdEncoding.EncodeToString(audio)
		audioDelta := map[string]interface{}{
			"event":     "media",
			"streamSid": streamSid,
			"media": map[string]string{
				"payload": audioPayload,
			},
		}

		if err := t.conn.WriteJSON(audioDelta); err != nil {
			// Handle websocket write error
			return
		}
	}()
}

func (t *TwilioAudioInterface) Interrupt() {
	go func() {
		t.mu.Lock()
		streamSid := t.streamSid
		t.mu.Unlock()

		if streamSid == "" {
			return
		}

		clearMessage := map[string]interface{}{
			"event":     "clear",
			"streamSid": streamSid,
		}

		if err := t.conn.WriteJSON(clearMessage); err != nil {
			// Handle websocket write error
			return
		}
	}()
}

func (t *TwilioAudioInterface) HandleTwilioMessage(data map[string]interface{}) {
	eventType, ok := data["event"].(string)
	if !ok {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch eventType {
	case "start":
		if start, ok := data["start"].(map[string]interface{}); ok {
			if streamSid, ok := start["streamSid"].(string); ok {
				t.streamSid = streamSid
			}
		}
	case "media":
		if t.inputCallback == nil {
			return
		}

		if media, ok := data["media"].(map[string]interface{}); ok {
			if payload, ok := media["payload"].(string); ok {
				audioData, err := base64.StdEncoding.DecodeString(payload)
				if err != nil {
					return
				}
				t.inputCallback(audioData)
			}
		}
	}
}
