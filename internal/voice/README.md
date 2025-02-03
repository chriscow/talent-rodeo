ased on my review of the voice package implementation, here's how the audio routing works:

Session Management:
- A Session manages the lifecycle of a voice chat interaction between Twilio and OpenAI
- It handles WebSocket connections and coordinates audio streaming
- Uses a Coordinator to manage bidirectional audio streams

Audio Flow:
- Twilio sends audio through WebSocket (base64 encoded)
- Audio is processed through input pipeline
- Audio is sent to OpenAI for processing
- OpenAI responses are processed through output pipeline
- Processed audio is sent back to Twilio

Components:
- Coordinator: Manages audio processing pipelines
OpenAIClient: Handles OpenAI WebSocket connection and audio streaming
TwilioAudioWriter: Handles sending audio back to Twilio
AudioProcessor: Interface for audio processing (silence detection, recording)   