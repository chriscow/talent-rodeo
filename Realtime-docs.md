Realtime
Beta
Communicate with a GPT-4o class model live, in real time, over WebSocket. Produces both audio and text transcriptions. Learn more about the Realtime API.
Client events
These are events that the OpenAI Realtime WebSocket server will accept from the client.
session.update
Send this event to update the session’s default configuration. The client may send this event at any time to update the session configuration, and any field may be updated at any time, except for "voice". The server will respond with a session.updated event that shows the full effective configuration. Only fields that are present are updated, thus the correct way to clear a field like "instructions" is to pass an empty string.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be session.update.
session
object
Realtime session object configuration.

Show properties
OBJECT session.update

{
    "event_id": "event_123",
    "type": "session.update",
    "session": {
        "modalities": ["text", "audio"],
        "instructions": "You are a helpful assistant.",
        "voice": "sage",
        "input_audio_format": "pcm16",
        "output_audio_format": "pcm16",
        "input_audio_transcription": {
            "model": "whisper-1"
        },
        "turn_detection": {
            "type": "server_vad",
            "threshold": 0.5,
            "prefix_padding_ms": 300,
            "silence_duration_ms": 500
        },
        "tools": [
            {
                "type": "function",
                "name": "get_weather",
                "description": "Get the current weather...",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "location": { "type": "string" }
                    },
                    "required": ["location"]
                }
            }
        ],
        "tool_choice": "auto",
        "temperature": 0.8,
        "max_response_output_tokens": "inf"
    }
}
input_audio_buffer.append
Send this event to append audio bytes to the input audio buffer. The audio buffer is temporary storage you can write to and later commit. In Server VAD mode, the audio buffer is used to detect speech and the server will decide when to commit. When Server VAD is disabled, you must commit the audio buffer manually.

The client may choose how much audio to place in each event up to a maximum of 15 MiB, for example streaming smaller chunks from the client may allow the VAD to be more responsive. Unlike made other client events, the server will not send a confirmation response to this event.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be input_audio_buffer.append.
audio
string
Base64-encoded audio bytes. This must be in the format specified by the input_audio_format field in the session configuration.
OBJECT input_audio_buffer.append

{
    "event_id": "event_456",
    "type": "input_audio_buffer.append",
    "audio": "Base64EncodedAudioData"
}
input_audio_buffer.commit
Send this event to commit the user input audio buffer, which will create a new user message item in the conversation. This event will produce an error if the input audio buffer is empty. When in Server VAD mode, the client does not need to send this event, the server will commit the audio buffer automatically.

Committing the input audio buffer will trigger input audio transcription (if enabled in session configuration), but it will not create a response from the model. The server will respond with an input_audio_buffer.committed event.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be input_audio_buffer.commit.
OBJECT input_audio_buffer.commit

{
    "event_id": "event_789",
    "type": "input_audio_buffer.commit"
}
input_audio_buffer.clear
Send this event to clear the audio bytes in the buffer. The server will respond with an input_audio_buffer.cleared event.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be input_audio_buffer.clear.
OBJECT input_audio_buffer.clear

{
    "event_id": "event_012",
    "type": "input_audio_buffer.clear"
}
conversation.item.create
Add a new Item to the Conversation's context, including messages, function calls, and function call responses. This event can be used both to populate a "history" of the conversation and to add new items mid-stream, but has the current limitation that it cannot populate assistant audio messages.

If successful, the server will respond with a conversation.item.created event, otherwise an error event will be sent.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be conversation.item.create.
previous_item_id
string
The ID of the preceding item after which the new item will be inserted. If not set, the new item will be appended to the end of the conversation. If set, it allows an item to be inserted mid-conversation. If the ID cannot be found, an error will be returned and the item will not be added.
item
object
The item to add to the conversation.

Show properties
OBJECT conversation.item.create

{
    "event_id": "event_345",
    "type": "conversation.item.create",
    "previous_item_id": null,
    "item": {
        "id": "msg_001",
        "type": "message",
        "role": "user",
        "content": [
            {
                "type": "input_text",
                "text": "Hello, how are you?"
            }
        ]
    }
}
conversation.item.truncate
Send this event to truncate a previous assistant message’s audio. The server will produce audio faster than realtime, so this event is useful when the user interrupts to truncate audio that has already been sent to the client but not yet played. This will synchronize the server's understanding of the audio with the client's playback.

Truncating audio will delete the server-side text transcript to ensure there is not text in the context that hasn't been heard by the user.

If successful, the server will respond with a conversation.item.truncated event.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be conversation.item.truncate.
item_id
string
The ID of the assistant message item to truncate. Only assistant message items can be truncated.
content_index
integer
The index of the content part to truncate. Set this to 0.
audio_end_ms
integer
Inclusive duration up to which audio is truncated, in milliseconds. If the audio_end_ms is greater than the actual audio duration, the server will respond with an error.
OBJECT conversation.item.truncate

{
    "event_id": "event_678",
    "type": "conversation.item.truncate",
    "item_id": "msg_002",
    "content_index": 0,
    "audio_end_ms": 1500
}
conversation.item.delete
Send this event when you want to remove any item from the conversation history. The server will respond with a conversation.item.deleted event, unless the item does not exist in the conversation history, in which case the server will respond with an error.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be conversation.item.delete.
item_id
string
The ID of the item to delete.
OBJECT conversation.item.delete

{
    "event_id": "event_901",
    "type": "conversation.item.delete",
    "item_id": "msg_003"
}
response.create
This event instructs the server to create a Response, which means triggering model inference. When in Server VAD mode, the server will create Responses automatically.

A Response will include at least one Item, and may have two, in which case the second will be a function call. These Items will be appended to the conversation history.

The server will respond with a response.created event, events for Items and content created, and finally a response.done event to indicate the Response is complete.

The response.create event includes inference configuration like instructions, and temperature. These fields will override the Session's configuration for this Response only.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be response.create.
response
object
Realtime session object configuration.

Show properties
OBJECT response.create

{
    "event_id": "event_234",
    "type": "response.create",
    "response": {
        "modalities": ["text", "audio"],
        "instructions": "Please assist the user.",
        "voice": "sage",
        "output_audio_format": "pcm16",
        "tools": [
            {
                "type": "function",
                "name": "calculate_sum",
                "description": "Calculates the sum of two numbers.",
                "parameters": {
                    "type": "object",
                    "properties": {
                        "a": { "type": "number" },
                        "b": { "type": "number" }
                    },
                    "required": ["a", "b"]
                }
            }
        ],
        "tool_choice": "auto",
        "temperature": 0.7,
        "max_output_tokens": 150
    }
}
response.cancel
Send this event to cancel an in-progress response. The server will respond with a response.cancelled event or an error if there is no response to cancel.

event_id
string
Optional client-generated ID used to identify this event.
type
string
The event type, must be response.cancel.
OBJECT response.cancel

{
    "event_id": "event_567",
    "type": "response.cancel"
}
Server events
These are events emitted from the OpenAI Realtime WebSocket server to the client.
error
Returned when an error occurs, which could be a client problem or a server problem. Most errors are recoverable and the session will stay open, we recommend to implementors to monitor and log error messages by default.

event_id
string
The unique ID of the server event.
type
string
The event type, must be error.
error
object
Details of the error.

Show properties
OBJECT error

{
    "event_id": "event_890",
    "type": "error",
    "error": {
        "type": "invalid_request_error",
        "code": "invalid_event",
        "message": "The 'type' field is missing.",
        "param": null,
        "event_id": "event_567"
    }
}
session.created
Returned when a Session is created. Emitted automatically when a new connection is established as the first server event. This event will contain the default Session configuration.

event_id
string
The unique ID of the server event.
type
string
The event type, must be session.created.
session
object
Realtime session object configuration.

Show properties
OBJECT session.created

{
    "event_id": "event_1234",
    "type": "session.created",
    "session": {
        "id": "sess_001",
        "object": "realtime.session",
        "model": "gpt-4o-realtime-preview-2024-10-01",
        "modalities": ["text", "audio"],
        "instructions": "...model instructions here...",
        "voice": "sage",
        "input_audio_format": "pcm16",
        "output_audio_format": "pcm16",
        "input_audio_transcription": null,
        "turn_detection": {
            "type": "server_vad",
            "threshold": 0.5,
            "prefix_padding_ms": 300,
            "silence_duration_ms": 200
        },
        "tools": [],
        "tool_choice": "auto",
        "temperature": 0.8,
        "max_response_output_tokens": "inf"
    }
}
session.updated
Returned when a session is updated with a session.update event, unless there is an error.

event_id
string
The unique ID of the server event.
type
string
The event type, must be session.updated.
session
object
Realtime session object configuration.

Show properties
OBJECT session.updated

{
    "event_id": "event_5678",
    "type": "session.updated",
    "session": {
        "id": "sess_001",
        "object": "realtime.session",
        "model": "gpt-4o-realtime-preview-2024-10-01",
        "modalities": ["text"],
        "instructions": "New instructions",
        "voice": "sage",
        "input_audio_format": "pcm16",
        "output_audio_format": "pcm16",
        "input_audio_transcription": {
            "model": "whisper-1"
        },
        "turn_detection": null,
        "tools": [],
        "tool_choice": "none",
        "temperature": 0.7,
        "max_response_output_tokens": 200
    }
}
conversation.created
Returned when a conversation is created. Emitted right after session creation.

event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.created.
conversation
object
The conversation resource.

Show properties
OBJECT conversation.created

{
    "event_id": "event_9101",
    "type": "conversation.created",
    "conversation": {
        "id": "conv_001",
        "object": "realtime.conversation"
    }
}
conversation.item.created
Returned when a conversation item is created. There are several scenarios that produce this event:

The server is generating a Response, which if successful will produce either one or two Items, which will be of type message (role assistant) or type function_call.
The input audio buffer has been committed, either by the client or the server (in server_vad mode). The server will take the content of the input audio buffer and add it to a new user message Item.
The client has sent a conversation.item.create event to add a new Item to the Conversation.
event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.item.created.
previous_item_id
string
The ID of the preceding item in the Conversation context, allows the client to understand the order of the conversation.
item
object
The item to add to the conversation.

Show properties
OBJECT conversation.item.created

{
    "event_id": "event_1920",
    "type": "conversation.item.created",
    "previous_item_id": "msg_002",
    "item": {
        "id": "msg_003",
        "object": "realtime.item",
        "type": "message",
        "status": "completed",
        "role": "user",
        "content": [
            {
                "type": "input_audio",
                "transcript": "hello how are you",
                "audio": "base64encodedaudio=="
            }
        ]
    }
}
conversation.item.input_audio_transcription.completed
This event is the output of audio transcription for user audio written to the user audio buffer. Transcription begins when the input audio buffer is committed by the client or server (in server_vad mode). Transcription runs asynchronously with Response creation, so this event may come before or after the Response events.

Realtime API models accept audio natively, and thus input transcription is a separate process run on a separate ASR (Automatic Speech Recognition) model, currently always whisper-1. Thus the transcript may diverge somewhat from the model's interpretation, and should be treated as a rough guide.

event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.item.input_audio_transcription.completed.
item_id
string
The ID of the user message item containing the audio.
content_index
integer
The index of the content part containing the audio.
transcript
string
The transcribed text.
OBJECT conversation.item.input_audio_transcription.completed

{
    "event_id": "event_2122",
    "type": "conversation.item.input_audio_transcription.completed",
    "item_id": "msg_003",
    "content_index": 0,
    "transcript": "Hello, how are you?"
}
conversation.item.input_audio_transcription.failed
Returned when input audio transcription is configured, and a transcription request for a user message failed. These events are separate from other error events so that the client can identify the related Item.

event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.item.input_audio_transcription.failed.
item_id
string
The ID of the user message item.
content_index
integer
The index of the content part containing the audio.
error
object
Details of the transcription error.

Show properties
OBJECT conversation.item.input_audio_transcription.failed

{
    "event_id": "event_2324",
    "type": "conversation.item.input_audio_transcription.failed",
    "item_id": "msg_003",
    "content_index": 0,
    "error": {
        "type": "transcription_error",
        "code": "audio_unintelligible",
        "message": "The audio could not be transcribed.",
        "param": null
    }
}
conversation.item.truncated
Returned when an earlier assistant audio message item is truncated by the client with a conversation.item.truncate event. This event is used to synchronize the server's understanding of the audio with the client's playback.

This action will truncate the audio and remove the server-side text transcript to ensure there is no text in the context that hasn't been heard by the user.

event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.item.truncated.
item_id
string
The ID of the assistant message item that was truncated.
content_index
integer
The index of the content part that was truncated.
audio_end_ms
integer
The duration up to which the audio was truncated, in milliseconds.
OBJECT conversation.item.truncated

{
    "event_id": "event_2526",
    "type": "conversation.item.truncated",
    "item_id": "msg_004",
    "content_index": 0,
    "audio_end_ms": 1500
}
conversation.item.deleted
Returned when an item in the conversation is deleted by the client with a conversation.item.delete event. This event is used to synchronize the server's understanding of the conversation history with the client's view.

event_id
string
The unique ID of the server event.
type
string
The event type, must be conversation.item.deleted.
item_id
string
The ID of the item that was deleted.
OBJECT conversation.item.deleted

{
    "event_id": "event_2728",
    "type": "conversation.item.deleted",
    "item_id": "msg_005"
}
input_audio_buffer.committed
Returned when an input audio buffer is committed, either by the client or automatically in server VAD mode. The item_id property is the ID of the user message item that will be created, thus a conversation.item.created event will also be sent to the client.

event_id
string
The unique ID of the server event.
type
string
The event type, must be input_audio_buffer.committed.
previous_item_id
string
The ID of the preceding item after which the new item will be inserted.
item_id
string
The ID of the user message item that will be created.
OBJECT input_audio_buffer.committed

{
    "event_id": "event_1121",
    "type": "input_audio_buffer.committed",
    "previous_item_id": "msg_001",
    "item_id": "msg_002"
}
input_audio_buffer.cleared
Returned when the input audio buffer is cleared by the client with a input_audio_buffer.clear event.

event_id
string
The unique ID of the server event.
type
string
The event type, must be input_audio_buffer.cleared.
OBJECT input_audio_buffer.cleared

{
    "event_id": "event_1314",
    "type": "input_audio_buffer.cleared"
}
input_audio_buffer.speech_started
Sent by the server when in server_vad mode to indicate that speech has been detected in the audio buffer. This can happen any time audio is added to the buffer (unless speech is already detected). The client may want to use this event to interrupt audio playback or provide visual feedback to the user.

The client should expect to receive a input_audio_buffer.speech_stopped event when speech stops. The item_id property is the ID of the user message item that will be created when speech stops and will also be included in the input_audio_buffer.speech_stopped event (unless the client manually commits the audio buffer during VAD activation).

event_id
string
The unique ID of the server event.
type
string
The event type, must be input_audio_buffer.speech_started.
audio_start_ms
integer
Milliseconds from the start of all audio written to the buffer during the session when speech was first detected. This will correspond to the beginning of audio sent to the model, and thus includes the prefix_padding_ms configured in the Session.
item_id
string
The ID of the user message item that will be created when speech stops.
OBJECT input_audio_buffer.speech_started

{
    "event_id": "event_1516",
    "type": "input_audio_buffer.speech_started",
    "audio_start_ms": 1000,
    "item_id": "msg_003"
}
input_audio_buffer.speech_stopped
Returned in server_vad mode when the server detects the end of speech in the audio buffer. The server will also send an conversation.item.created event with the user message item that is created from the audio buffer.

event_id
string
The unique ID of the server event.
type
string
The event type, must be input_audio_buffer.speech_stopped.
audio_end_ms
integer
Milliseconds since the session started when speech stopped. This will correspond to the end of audio sent to the model, and thus includes the min_silence_duration_ms configured in the Session.
item_id
string
The ID of the user message item that will be created.
OBJECT input_audio_buffer.speech_stopped

{
    "event_id": "event_1718",
    "type": "input_audio_buffer.speech_stopped",
    "audio_end_ms": 2000,
    "item_id": "msg_003"
}
response.created
Returned when a new Response is created. The first event of response creation, where the response is in an initial state of in_progress.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.created.
response
object
The response resource.

Show properties
OBJECT response.created

{
    "event_id": "event_2930",
    "type": "response.created",
    "response": {
        "id": "resp_001",
        "object": "realtime.response",
        "status": "in_progress",
        "status_details": null,
        "output": [],
        "usage": null
    }
}
response.done
Returned when a Response is done streaming. Always emitted, no matter the final state. The Response object included in the response.done event will include all output Items in the Response but will omit the raw audio data.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.done.
response
object
The response resource.

Show properties
OBJECT response.done

{
    "event_id": "event_3132",
    "type": "response.done",
    "response": {
        "id": "resp_001",
        "object": "realtime.response",
        "status": "completed",
        "status_details": null,
        "output": [
            {
                "id": "msg_006",
                "object": "realtime.item",
                "type": "message",
                "status": "completed",
                "role": "assistant",
                "content": [
                    {
                        "type": "text",
                        "text": "Sure, how can I assist you today?"
                    }
                ]
            }
        ],
        "usage": {
            "total_tokens":275,
            "input_tokens":127,
            "output_tokens":148,
            "input_token_details": {
                "cached_tokens":384,
                "text_tokens":119,
                "audio_tokens":8,
                "cached_tokens_details": {
                    "text_tokens": 128,
                    "audio_tokens": 256
                }
            },
            "output_token_details": {
              "text_tokens":36,
              "audio_tokens":112
            }
        }
    }
}
response.output_item.added
Returned when a new Item is created during Response generation.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.output_item.added.
response_id
string
The ID of the Response to which the item belongs.
output_index
integer
The index of the output item in the Response.
item
object
The item to add to the conversation.

Show properties
OBJECT response.output_item.added

{
    "event_id": "event_3334",
    "type": "response.output_item.added",
    "response_id": "resp_001",
    "output_index": 0,
    "item": {
        "id": "msg_007",
        "object": "realtime.item",
        "type": "message",
        "status": "in_progress",
        "role": "assistant",
        "content": []
    }
}
response.output_item.done
Returned when an Item is done streaming. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.output_item.done.
response_id
string
The ID of the Response to which the item belongs.
output_index
integer
The index of the output item in the Response.
item
object
The item to add to the conversation.

Show properties
OBJECT response.output_item.done

{
    "event_id": "event_3536",
    "type": "response.output_item.done",
    "response_id": "resp_001",
    "output_index": 0,
    "item": {
        "id": "msg_007",
        "object": "realtime.item",
        "type": "message",
        "status": "completed",
        "role": "assistant",
        "content": [
            {
                "type": "text",
                "text": "Sure, I can help with that."
            }
        ]
    }
}
response.content_part.added
Returned when a new content part is added to an assistant message item during response generation.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.content_part.added.
response_id
string
The ID of the response.
item_id
string
The ID of the item to which the content part was added.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
part
object
The content part that was added.

Show properties
OBJECT response.content_part.added

{
    "event_id": "event_3738",
    "type": "response.content_part.added",
    "response_id": "resp_001",
    "item_id": "msg_007",
    "output_index": 0,
    "content_index": 0,
    "part": {
        "type": "text",
        "text": ""
    }
}
response.content_part.done
Returned when a content part is done streaming in an assistant message item. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.content_part.done.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
part
object
The content part that is done.

Show properties
OBJECT response.content_part.done

{
    "event_id": "event_3940",
    "type": "response.content_part.done",
    "response_id": "resp_001",
    "item_id": "msg_007",
    "output_index": 0,
    "content_index": 0,
    "part": {
        "type": "text",
        "text": "Sure, I can help with that."
    }
}
response.text.delta
Returned when the text value of a "text" content part is updated.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.text.delta.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
delta
string
The text delta.
OBJECT response.text.delta

{
    "event_id": "event_4142",
    "type": "response.text.delta",
    "response_id": "resp_001",
    "item_id": "msg_007",
    "output_index": 0,
    "content_index": 0,
    "delta": "Sure, I can h"
}
response.text.done
Returned when the text value of a "text" content part is done streaming. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.text.done.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
text
string
The final text content.
OBJECT response.text.done

{
    "event_id": "event_4344",
    "type": "response.text.done",
    "response_id": "resp_001",
    "item_id": "msg_007",
    "output_index": 0,
    "content_index": 0,
    "text": "Sure, I can help with that."
}
response.audio_transcript.delta
Returned when the model-generated transcription of audio output is updated.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.audio_transcript.delta.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
delta
string
The transcript delta.
OBJECT response.audio_transcript.delta

{
    "event_id": "event_4546",
    "type": "response.audio_transcript.delta",
    "response_id": "resp_001",
    "item_id": "msg_008",
    "output_index": 0,
    "content_index": 0,
    "delta": "Hello, how can I a"
}
response.audio_transcript.done
Returned when the model-generated transcription of audio output is done streaming. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.audio_transcript.done.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
transcript
string
The final transcript of the audio.
OBJECT response.audio_transcript.done

{
    "event_id": "event_4748",
    "type": "response.audio_transcript.done",
    "response_id": "resp_001",
    "item_id": "msg_008",
    "output_index": 0,
    "content_index": 0,
    "transcript": "Hello, how can I assist you today?"
}
response.audio.delta
Returned when the model-generated audio is updated.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.audio.delta.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
delta
string
Base64-encoded audio data delta.
OBJECT response.audio.delta

{
    "event_id": "event_4950",
    "type": "response.audio.delta",
    "response_id": "resp_001",
    "item_id": "msg_008",
    "output_index": 0,
    "content_index": 0,
    "delta": "Base64EncodedAudioDelta"
}
response.audio.done
Returned when the model-generated audio is done. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.audio.done.
response_id
string
The ID of the response.
item_id
string
The ID of the item.
output_index
integer
The index of the output item in the response.
content_index
integer
The index of the content part in the item's content array.
OBJECT response.audio.done

{
    "event_id": "event_5152",
    "type": "response.audio.done",
    "response_id": "resp_001",
    "item_id": "msg_008",
    "output_index": 0,
    "content_index": 0
}
response.function_call_arguments.delta
Returned when the model-generated function call arguments are updated.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.function_call_arguments.delta.
response_id
string
The ID of the response.
item_id
string
The ID of the function call item.
output_index
integer
The index of the output item in the response.
call_id
string
The ID of the function call.
delta
string
The arguments delta as a JSON string.
OBJECT response.function_call_arguments.delta

{
    "event_id": "event_5354",
    "type": "response.function_call_arguments.delta",
    "response_id": "resp_002",
    "item_id": "fc_001",
    "output_index": 0,
    "call_id": "call_001",
    "delta": "{\"location\": \"San\""
}
response.function_call_arguments.done
Returned when the model-generated function call arguments are done streaming. Also emitted when a Response is interrupted, incomplete, or cancelled.

event_id
string
The unique ID of the server event.
type
string
The event type, must be response.function_call_arguments.done.
response_id
string
The ID of the response.
item_id
string
The ID of the function call item.
output_index
integer
The index of the output item in the response.
call_id
string
The ID of the function call.
arguments
string
The final arguments as a JSON string.
OBJECT response.function_call_arguments.done

{
    "event_id": "event_5556",
    "type": "response.function_call_arguments.done",
    "response_id": "resp_002",
    "item_id": "fc_001",
    "output_index": 0,
    "call_id": "call_001",
    "arguments": "{\"location\": \"San Francisco\"}"
}
rate_limits.updated
Emitted at the beginning of a Response to indicate the updated rate limits. When a Response is created some tokens will be "reserved" for the output tokens, the rate limits shown here reflect that reservation, which is then adjusted accordingly once the Response is completed.

event_id
string
The unique ID of the server event.
type
string
The event type, must be rate_limits.updated.
rate_limits
array
List of rate limit information.

Show properties
OBJECT rate_limits.updated

{
    "event_id": "event_5758",
    "type": "rate_limits.updated",
    "rate_limits": [
        {
            "name": "requests",
            "limit": 1000,
            "remaining": 999,
            "reset_seconds": 60
        },
        {
            "name": "tokens",
            "limit": 50000,
            "remaining": 49950,
            "reset_seconds": 60
        }
    ]
}
Completions
Legacy
Given a prompt, the model will return one or more predicted completions along with the probabilities of alternative tokens at each position. Most developer should use our Chat Completions API to leverage our best and newest models.
Create completion
Legacy
POST
 
https://api.openai.com/v1/completions
Creates a completion for the provided prompt and parameters.

Request body

model
string
Required
ID of the model to use. You can use the List models API to see all of your available models, or see our Model overview for descriptions of them.
prompt
string or array
Required
The prompt(s) to generate completions for, encoded as a string, array of strings, array of tokens, or array of token arrays.

Note that <|endoftext|> is the document separator that the model sees during training, so if a prompt is not specified the model will generate as if from the beginning of a new document.
best_of
integer or null
Optional
Defaults to 1
Generates best_of completions server-side and returns the "best" (the one with the highest log probability per token). Results cannot be streamed.

When used with n, best_of controls the number of candidate completions and n specifies how many to return – best_of must be greater than n.

Note: Because this parameter generates many completions, it can quickly consume your token quota. Use carefully and ensure that you have reasonable settings for max_tokens and stop.
echo
boolean or null
Optional
Defaults to false
Echo back the prompt in addition to the completion
frequency_penalty
number or null
Optional
Defaults to 0
Number between -2.0 and 2.0. Positive values penalize new tokens based on their existing frequency in the text so far, decreasing the model's likelihood to repeat the same line verbatim.

See more information about frequency and presence penalties.
logit_bias
map
Optional
Defaults to null
Modify the likelihood of specified tokens appearing in the completion.

Accepts a JSON object that maps tokens (specified by their token ID in the GPT tokenizer) to an associated bias value from -100 to 100. You can use this tokenizer tool to convert text to token IDs. Mathematically, the bias is added to the logits generated by the model prior to sampling. The exact effect will vary per model, but values between -1 and 1 should decrease or increase likelihood of selection; values like -100 or 100 should result in a ban or exclusive selection of the relevant token.

As an example, you can pass {"50256": -100} to prevent the <|endoftext|> token from being generated.
logprobs
integer or null
Optional
Defaults to null
Include the log probabilities on the logprobs most likely output tokens, as well the chosen tokens. For example, if logprobs is 5, the API will return a list of the 5 most likely tokens. The API will always return the logprob of the sampled token, so there may be up to logprobs+1 elements in the response.

The maximum value for logprobs is 5.
max_tokens
integer or null
Optional
Defaults to 16
The maximum number of tokens that can be generated in the completion.

The token count of your prompt plus max_tokens cannot exceed the model's context length. Example Python code for counting tokens.
n
integer or null
Optional
Defaults to 1
How many completions to generate for each prompt.

Note: Because this parameter generates many completions, it can quickly consume your token quota. Use carefully and ensure that you have reasonable settings for max_tokens and stop.
presence_penalty
number or null
Optional
Defaults to 0
Number between -2.0 and 2.0. Positive values penalize new tokens based on whether they appear in the text so far, increasing the model's likelihood to talk about new topics.

See more information about frequency and presence penalties.
seed
integer or null
Optional
If specified, our system will make a best effort to sample deterministically, such that repeated requests with the same seed and parameters should return the same result.

Determinism is not guaranteed, and you should refer to the system_fingerprint response parameter to monitor changes in the backend.
stop
string / array / null
Optional
Defaults to null
Up to 4 sequences where the API will stop generating further tokens. The returned text will not contain the stop sequence.
stream
boolean or null
Optional
Defaults to false
Whether to stream back partial progress. If set, tokens will be sent as data-only server-sent events as they become available, with the stream terminated by a data: [DONE] message. Example Python code.
stream_options
object or null
Optional
Defaults to null
Options for streaming response. Only set this when you set stream: true.

Show properties
suffix
string or null
Optional
Defaults to null
The suffix that comes after a completion of inserted text.

This parameter is only supported for gpt-3.5-turbo-instruct.
temperature
number or null
Optional
Defaults to 1
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.

We generally recommend altering this or top_p but not both.
top_p
number or null
Optional
Defaults to 1
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
user
string
Optional
A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse. Learn more.
Returns

Returns a completion object, or a sequence of completion objects if the request is streamed.


No streaming

Streaming
Example request

from openai import OpenAI
client = OpenAI()

client.completions.create(
  model="gpt-3.5-turbo-instruct",
  prompt="Say this is a test",
  max_tokens=7,
  temperature=0
)
Response

{
  "id": "cmpl-uqkvlQyYK7bGYrRHQ0eXlWi7",
  "object": "text_completion",
  "created": 1589478378,
  "model": "gpt-3.5-turbo-instruct",
  "system_fingerprint": "fp_44709d6fcb",
  "choices": [
    {
      "text": "\n\nThis is indeed a test",
      "index": 0,
      "logprobs": null,
      "finish_reason": "length"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 7,
    "total_tokens": 12
  }
}
The completion object
Legacy
Represents a completion response from the API. Note: both the streamed and non-streamed response objects share the same shape (unlike the chat endpoint).

id
string
A unique identifier for the completion.
choices
array
The list of completion choices the model generated for the input prompt.

Show properties
created
integer
The Unix timestamp (in seconds) of when the completion was created.
model
string
The model used for completion.
system_fingerprint
string
This fingerprint represents the backend configuration that the model runs with.

Can be used in conjunction with the seed request parameter to understand when backend changes have been made that might impact determinism.
object
string
The object type, which is always "text_completion"
usage
object
Usage statistics for the completion request.

Show properties
OBJECT The completion object

{
  "id": "cmpl-uqkvlQyYK7bGYrRHQ0eXlWi7",
  "object": "text_completion",
  "created": 1589478378,
  "model": "gpt-4-turbo",
  "choices": [
    {
      "text": "\n\nThis is indeed a test",
      "index": 0,
      "logprobs": null,
      "finish_reason": "length"
    }
  ],
  "usage": {
    "prompt_tokens": 5,
    "completion_tokens": 7,
    "total_tokens": 12
  }
}
Assistants (v1)
Legacy
Build assistants that can call models and use tools to perform tasks.

Get started with the Assistants API
Create assistant (v1)
Legacy
POST
 
https://api.openai.com/v1/assistants
Create an assistant with a model and instructions.

Request body

model
string
Required
ID of the model to use. You can use the List models API to see all of your available models, or see our Model overview for descriptions of them. type: string
name
string or null
Optional
The name of the assistant. The maximum length is 256 characters.
description
string or null
Optional
The description of the assistant. The maximum length is 512 characters.
instructions
string or null
Optional
The system instructions that the assistant uses. The maximum length is 256,000 characters.
tools
array
Optional
Defaults to []
A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types code_interpreter, retrieval, or function.

Show possible types
file_ids
array
Optional
Defaults to []
A list of file IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order.
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
temperature
number or null
Optional
Defaults to 1
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
top_p
number or null
Optional
Defaults to 1
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
response_format
string or object
Optional
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
Returns

An assistant object.


Code Interpreter

Files
Example request

from openai import OpenAI
client = OpenAI()

my_assistant = client.beta.assistants.create(
    instructions="You are a personal math tutor. When asked a question, write and run Python code to answer the question.",
    name="Math Tutor",
    tools=[{"type": "code_interpreter"}],
    model="gpt-4-turbo",
)
print(my_assistant)
Response

{
  "id": "asst_abc123",
  "object": "assistant",
  "created_at": 1698984975,
  "name": "Math Tutor",
  "description": null,
  "model": "gpt-4-turbo",
  "instructions": "You are a personal math tutor. When asked a question, write and run Python code to answer the question.",
  "tools": [
    {
      "type": "code_interpreter"
    }
  ],
  "file_ids": [],
  "metadata": {},
  "top_p": 1.0,
  "temperature": 1.0,
  "response_format": "auto"
}
Create assistant file (v1)
Legacy
POST
 
https://api.openai.com/v1/assistants/{assistant_id}/files
Create an assistant file by attaching a File to an assistant.

Path parameters

assistant_id
string
Required
The ID of the assistant for which to create a File.
Request body

file_id
string
Required
A File ID (with purpose="assistants") that the assistant should use. Useful for tools like retrieval and code_interpreter that can access files.
Returns

An assistant file object.

Example request

from openai import OpenAI
client = OpenAI()

assistant_file = client.beta.assistants.files.create(
  assistant_id="asst_abc123",
  file_id="file-abc123"
)
print(assistant_file)
Response

{
  "id": "file-abc123",
  "object": "assistant.file",
  "created_at": 1699055364,
  "assistant_id": "asst_abc123"
}
List assistants (v1)
Legacy
GET
 
https://api.openai.com/v1/assistants
Returns a list of assistants.

Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
Returns

A list of assistant objects.

Example request

from openai import OpenAI
client = OpenAI()

my_assistants = client.beta.assistants.list(
    order="desc",
    limit="20",
)
print(my_assistants.data)
Response

{
  "object": "list",
  "data": [
    {
      "id": "asst_abc123",
      "object": "assistant",
      "created_at": 1698982736,
      "name": "Coding Tutor",
      "description": null,
      "model": "gpt-4-turbo",
      "instructions": "You are a helpful assistant designed to make me better at coding!",
      "tools": [],
      "file_ids": [],
      "metadata": {},
      "top_p": 1.0,
      "temperature": 1.0,
      "response_format": "auto"
    },
    {
      "id": "asst_abc456",
      "object": "assistant",
      "created_at": 1698982718,
      "name": "My Assistant",
      "description": null,
      "model": "gpt-4-turbo",
      "instructions": "You are a helpful assistant designed to make me better at coding!",
      "tools": [],
      "file_ids": [],
      "metadata": {},
      "top_p": 1.0,
      "temperature": 1.0,
      "response_format": "auto"
    },
    {
      "id": "asst_abc789",
      "object": "assistant",
      "created_at": 1698982643,
      "name": null,
      "description": null,
      "model": "gpt-4-turbo",
      "instructions": null,
      "tools": [],
      "file_ids": [],
      "metadata": {},
      "top_p": 1.0,
      "temperature": 1.0,
      "response_format": "auto"
    }
  ],
  "first_id": "asst_abc123",
  "last_id": "asst_abc789",
  "has_more": false
}
List assistant files (v1)
Legacy
GET
 
https://api.openai.com/v1/assistants/{assistant_id}/files
Returns a list of assistant files.

Path parameters

assistant_id
string
Required
The ID of the assistant the file belongs to.
Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
Returns

A list of assistant file objects.

Example request

from openai import OpenAI
client = OpenAI()

assistant_files = client.beta.assistants.files.list(
  assistant_id="asst_abc123"
)
print(assistant_files)
Response

{
  "object": "list",
  "data": [
    {
      "id": "file-abc123",
      "object": "assistant.file",
      "created_at": 1699060412,
      "assistant_id": "asst_abc123"
    },
    {
      "id": "file-abc456",
      "object": "assistant.file",
      "created_at": 1699060412,
      "assistant_id": "asst_abc123"
    }
  ],
  "first_id": "file-abc123",
  "last_id": "file-abc456",
  "has_more": false
}
Retrieve assistant (v1)
Legacy
GET
 
https://api.openai.com/v1/assistants/{assistant_id}
Retrieves an assistant.

Path parameters

assistant_id
string
Required
The ID of the assistant to retrieve.
Returns

The assistant object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

my_assistant = client.beta.assistants.retrieve("asst_abc123")
print(my_assistant)
Response

{
  "id": "asst_abc123",
  "object": "assistant",
  "created_at": 1699009709,
  "name": "HR Helper",
  "description": null,
  "model": "gpt-4-turbo",
  "instructions": "You are an HR bot, and you have access to files to answer employee questions about company policies.",
  "tools": [
    {
      "type": "retrieval"
    }
  ],
  "file_ids": [
    "file-abc123"
  ],
  "metadata": {}
}
Retrieve assistant file (v1)
Legacy
GET
 
https://api.openai.com/v1/assistants/{assistant_id}/files/{file_id}
Retrieves an AssistantFile.

Path parameters

assistant_id
string
Required
The ID of the assistant who the file belongs to.
file_id
string
Required
The ID of the file we're getting.
Returns

The assistant file object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

assistant_file = client.beta.assistants.files.retrieve(
  assistant_id="asst_abc123",
  file_id="file-abc123"
)
print(assistant_file)
Response

{
  "id": "file-abc123",
  "object": "assistant.file",
  "created_at": 1699055364,
  "assistant_id": "asst_abc123"
}
Modify assistant (v1)
Legacy
POST
 
https://api.openai.com/v1/assistants/{assistant_id}
Modifies an assistant.

Path parameters

assistant_id
string
Required
The ID of the assistant to modify.
Request body

model
Optional
ID of the model to use. You can use the List models API to see all of your available models, or see our Model overview for descriptions of them. type: string
name
string or null
Optional
The name of the assistant. The maximum length is 256 characters.
description
string or null
Optional
The description of the assistant. The maximum length is 512 characters.
instructions
string or null
Optional
The system instructions that the assistant uses. The maximum length is 256,000 characters.
tools
array
Optional
Defaults to []
A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types code_interpreter, retrieval, or function.

Show possible types
file_ids
array
Optional
Defaults to []
A list of File IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order. If a file was previously attached to the list but does not show up in the list, it will be deleted from the assistant.
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
temperature
number or null
Optional
Defaults to 1
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
top_p
number or null
Optional
Defaults to 1
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
response_format
string or object
Optional
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
Returns

The modified assistant object.

Example request

from openai import OpenAI
client = OpenAI()

my_updated_assistant = client.beta.assistants.update(
  "asst_abc123",
  instructions="You are an HR bot, and you have access to files to answer employee questions about company policies. Always response with info from either of the files.",
  name="HR Helper",
  tools=[{"type": "retrieval"}],
  model="gpt-4-turbo",
  file_ids=["file-abc123", "file-abc456"],
)

print(my_updated_assistant)
Response

{
  "id": "asst_abc123",
  "object": "assistant",
  "created_at": 1699009709,
  "name": "HR Helper",
  "description": null,
  "model": "gpt-4-turbo",
  "instructions": "You are an HR bot, and you have access to files to answer employee questions about company policies. Always response with info from either of the files.",
  "tools": [
    {
      "type": "retrieval"
    }
  ],
  "file_ids": [
    "file-abc123",
    "file-abc456"
  ],
  "metadata": {},
  "top_p": 1.0,
  "temperature": 1.0,
  "response_format": "auto"
}
Delete assistant (v1)
Legacy
DELETE
 
https://api.openai.com/v1/assistants/{assistant_id}
Delete an assistant.

Path parameters

assistant_id
string
Required
The ID of the assistant to delete.
Returns

Deletion status

Example request

from openai import OpenAI
client = OpenAI()

response = client.beta.assistants.delete("asst_abc123")
print(response)
Response

{
  "id": "asst_abc123",
  "object": "assistant.deleted",
  "deleted": true
}
Delete assistant file (v1)
Legacy
DELETE
 
https://api.openai.com/v1/assistants/{assistant_id}/files/{file_id}
Delete an assistant file.

Path parameters

assistant_id
string
Required
The ID of the assistant that the file belongs to.
file_id
string
Required
The ID of the file to delete.
Returns

Deletion status

Example request

from openai import OpenAI
client = OpenAI()

deleted_assistant_file = client.beta.assistants.files.delete(
    assistant_id="asst_abc123",
    file_id="file-abc123"
)
print(deleted_assistant_file)
Response

{
  id: "file-abc123",
  object: "assistant.file.deleted",
  deleted: true
}
The assistant object (v1)
Legacy
Represents an assistant that can call the model and use tools.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always assistant.
created_at
integer
The Unix timestamp (in seconds) for when the assistant was created.
name
string or null
The name of the assistant. The maximum length is 256 characters.
description
string or null
The description of the assistant. The maximum length is 512 characters.
model
ID of the model to use. You can use the List models API to see all of your available models, or see our Model overview for descriptions of them. type: string
instructions
string or null
The system instructions that the assistant uses. The maximum length is 256,000 characters.
tools
array
A list of tool enabled on the assistant. There can be a maximum of 128 tools per assistant. Tools can be of types code_interpreter, retrieval, or function.

Show possible types
file_ids
array
A list of file IDs attached to this assistant. There can be a maximum of 20 files attached to the assistant. Files are ordered by their creation date in ascending order.
metadata
map
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
temperature
number or null
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
top_p
number or null
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
response_format
string or object
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
OBJECT The assistant object (v1)

{
  "id": "asst_abc123",
  "object": "assistant",
  "created_at": 1698984975,
  "name": "Math Tutor",
  "description": null,
  "model": "gpt-4-turbo",
  "instructions": "You are a personal math tutor. When asked a question, write and run Python code to answer the question.",
  "tools": [
    {
      "type": "code_interpreter"
    }
  ],
  "file_ids": [],
  "metadata": {},
  "top_p": 1.0,
  "temperature": 1.0,
  "response_format": "auto"
}
The assistant file object (v1)
Legacy
A list of Files attached to an assistant.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always assistant.file.
created_at
integer
The Unix timestamp (in seconds) for when the assistant file was created.
assistant_id
string
The assistant ID that the file is attached to.
OBJECT The assistant file object (v1)

{
  "id": "file-abc123",
  "object": "assistant.file",
  "created_at": 1699055364,
  "assistant_id": "asst_abc123"
}
Threads (v1)
Legacy
Create threads that assistants can interact with.

Related guide: Assistants
Create thread (v1)
Legacy
POST
 
https://api.openai.com/v1/threads
Create a thread.

Request body

messages
array
Optional
A list of messages to start the thread with.

Show properties
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
Returns

A thread object.


Empty

Messages
Example request

from openai import OpenAI
client = OpenAI()

empty_thread = client.beta.threads.create()
print(empty_thread)
Response

{
  "id": "thread_abc123",
  "object": "thread",
  "created_at": 1699012949,
  "metadata": {}
}
Retrieve thread (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}
Retrieves a thread.

Path parameters

thread_id
string
Required
The ID of the thread to retrieve.
Returns

The thread object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

my_thread = client.beta.threads.retrieve("thread_abc123")
print(my_thread)
Response

{
  "id": "thread_abc123",
  "object": "thread",
  "created_at": 1699014083,
  "metadata": {}
}
Modify thread (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}
Modifies a thread.

Path parameters

thread_id
string
Required
The ID of the thread to modify. Only the metadata can be modified.
Request body

metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
Returns

The modified thread object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

my_updated_thread = client.beta.threads.update(
  "thread_abc123",
  metadata={
    "modified": "true",
    "user": "abc123"
  }
)
print(my_updated_thread)
Response

{
  "id": "thread_abc123",
  "object": "thread",
  "created_at": 1699014083,
  "metadata": {
    "modified": "true",
    "user": "abc123"
  }
}
Delete thread (v1)
Legacy
DELETE
 
https://api.openai.com/v1/threads/{thread_id}
Delete a thread.

Path parameters

thread_id
string
Required
The ID of the thread to delete.
Returns

Deletion status

Example request

from openai import OpenAI
client = OpenAI()

response = client.beta.threads.delete("thread_abc123")
print(response)
Response

{
  "id": "thread_abc123",
  "object": "thread.deleted",
  "deleted": true
}
The thread object (v1)
Legacy
Represents a thread that contains messages.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always thread.
created_at
integer
The Unix timestamp (in seconds) for when the thread was created.
metadata
map
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
OBJECT The thread object (v1)

{
  "id": "thread_abc123",
  "object": "thread",
  "created_at": 1698107661,
  "metadata": {}
}
Messages (v1)
Legacy
Create messages within threads

Related guide: Assistants
Create message (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/messages
Create a message.

Path parameters

thread_id
string
Required
The ID of the thread to create a message for.
Request body

role
string
Required
The role of the entity that is creating the message. Allowed values include:

user: Indicates the message is sent by an actual user and should be used in most cases to represent user-generated messages.
assistant: Indicates the message is generated by the assistant. Use this value to insert messages from the assistant into the conversation.
content
string
Required
The content of the message.
file_ids
array
Optional
Defaults to []
A list of File IDs that the message should use. There can be a maximum of 10 files attached to a message. Useful for tools like retrieval and code_interpreter that can access and use files.
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
Returns

A message object.

Example request

from openai import OpenAI
client = OpenAI()

thread_message = client.beta.threads.messages.create(
  "thread_abc123",
  role="user",
  content="How does AI work? Explain it in simple terms.",
)
print(thread_message)
Response

{
  "id": "msg_abc123",
  "object": "thread.message",
  "created_at": 1699017614,
  "thread_id": "thread_abc123",
  "role": "user",
  "content": [
    {
      "type": "text",
      "text": {
        "value": "How does AI work? Explain it in simple terms.",
        "annotations": []
      }
    }
  ],
  "file_ids": [],
  "assistant_id": null,
  "run_id": null,
  "metadata": {}
}
List messages (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/messages
Returns a list of messages for a given thread.

Path parameters

thread_id
string
Required
The ID of the thread the messages belong to.
Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
run_id
string
Optional
Filter messages by the run ID that generated them.
Returns

A list of message objects.

Example request

from openai import OpenAI
client = OpenAI()

thread_messages = client.beta.threads.messages.list("thread_abc123")
print(thread_messages.data)
Response

{
  "object": "list",
  "data": [
    {
      "id": "msg_abc123",
      "object": "thread.message",
      "created_at": 1699016383,
      "thread_id": "thread_abc123",
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": {
            "value": "How does AI work? Explain it in simple terms.",
            "annotations": []
          }
        }
      ],
      "file_ids": [],
      "assistant_id": null,
      "run_id": null,
      "metadata": {}
    },
    {
      "id": "msg_abc456",
      "object": "thread.message",
      "created_at": 1699016383,
      "thread_id": "thread_abc123",
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": {
            "value": "Hello, what is AI?",
            "annotations": []
          }
        }
      ],
      "file_ids": [
        "file-abc123"
      ],
      "assistant_id": null,
      "run_id": null,
      "metadata": {}
    }
  ],
  "first_id": "msg_abc123",
  "last_id": "msg_abc456",
  "has_more": false
}
List message files (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/messages/{message_id}/files
Returns a list of message files.

Path parameters

thread_id
string
Required
The ID of the thread that the message and files belong to.
message_id
string
Required
The ID of the message that the files belongs to.
Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
Returns

A list of message file objects.

Example request

from openai import OpenAI
client = OpenAI()

message_files = client.beta.threads.messages.files.list(
  thread_id="thread_abc123",
  message_id="msg_abc123"
)
print(message_files)
Response

{
  "object": "list",
  "data": [
    {
      "id": "file-abc123",
      "object": "thread.message.file",
      "created_at": 1699061776,
      "message_id": "msg_abc123"
    },
    {
      "id": "file-abc123",
      "object": "thread.message.file",
      "created_at": 1699061776,
      "message_id": "msg_abc123"
    }
  ],
  "first_id": "file-abc123",
  "last_id": "file-abc123",
  "has_more": false
}
Retrieve message (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/messages/{message_id}
Retrieve a message.

Path parameters

thread_id
string
Required
The ID of the thread to which this message belongs.
message_id
string
Required
The ID of the message to retrieve.
Returns

The message object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

message = client.beta.threads.messages.retrieve(
  message_id="msg_abc123",
  thread_id="thread_abc123",
)
print(message)
Response

{
  "id": "msg_abc123",
  "object": "thread.message",
  "created_at": 1699017614,
  "thread_id": "thread_abc123",
  "role": "user",
  "content": [
    {
      "type": "text",
      "text": {
        "value": "How does AI work? Explain it in simple terms.",
        "annotations": []
      }
    }
  ],
  "file_ids": [],
  "assistant_id": null,
  "run_id": null,
  "metadata": {}
}
Retrieve message file (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/messages/{message_id}/files/{file_id}
Retrieves a message file.

Path parameters

thread_id
string
Required
The ID of the thread to which the message and File belong.
message_id
string
Required
The ID of the message the file belongs to.
file_id
string
Required
The ID of the file being retrieved.
Returns

The message file object.

Example request

from openai import OpenAI
client = OpenAI()

message_files = client.beta.threads.messages.files.retrieve(
    thread_id="thread_abc123",
    message_id="msg_abc123",
    file_id="file-abc123"
)
print(message_files)
Response

{
  "id": "file-abc123",
  "object": "thread.message.file",
  "created_at": 1699061776,
  "message_id": "msg_abc123"
}
Modify message (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/messages/{message_id}
Modifies a message.

Path parameters

thread_id
string
Required
The ID of the thread to which this message belongs.
message_id
string
Required
The ID of the message to modify.
Request body

metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
Returns

The modified message object.

Example request

from openai import OpenAI
client = OpenAI()

message = client.beta.threads.messages.update(
  message_id="msg_abc12",
  thread_id="thread_abc123",
  metadata={
    "modified": "true",
    "user": "abc123",
  },
)
print(message)
Response

{
  "id": "msg_abc123",
  "object": "thread.message",
  "created_at": 1699017614,
  "thread_id": "thread_abc123",
  "role": "user",
  "content": [
    {
      "type": "text",
      "text": {
        "value": "How does AI work? Explain it in simple terms.",
        "annotations": []
      }
    }
  ],
  "file_ids": [],
  "assistant_id": null,
  "run_id": null,
  "metadata": {
    "modified": "true",
    "user": "abc123"
  }
}
The message object (v1)
Legacy
Represents a message within a thread.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always thread.message.
created_at
integer
The Unix timestamp (in seconds) for when the message was created.
thread_id
string
The thread ID that this message belongs to.
status
string
The status of the message, which can be either in_progress, incomplete, or completed.
incomplete_details
object or null
On an incomplete message, details about why the message is incomplete.

Show properties
completed_at
integer or null
The Unix timestamp (in seconds) for when the message was completed.
incomplete_at
integer or null
The Unix timestamp (in seconds) for when the message was marked as incomplete.
role
string
The entity that produced the message. One of user or assistant.
content
array
The content of the message in array of text and/or images.

Show possible types
assistant_id
string or null
If applicable, the ID of the assistant that authored this message.
run_id
string or null
The ID of the run associated with the creation of this message. Value is null when messages are created manually using the create message or create thread endpoints.
file_ids
array
A list of file IDs that the assistant should use. Useful for tools like retrieval and code_interpreter that can access files. A maximum of 10 files can be attached to a message.
metadata
map
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
OBJECT The message object (v1)

{
  "id": "msg_abc123",
  "object": "thread.message",
  "created_at": 1698983503,
  "thread_id": "thread_abc123",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": {
        "value": "Hi! How can I help you today?",
        "annotations": []
      }
    }
  ],
  "file_ids": [],
  "assistant_id": "asst_abc123",
  "run_id": "run_abc123",
  "metadata": {}
}
The message file object (v1)
Legacy
A list of files attached to a message.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always thread.message.file.
created_at
integer
The Unix timestamp (in seconds) for when the message file was created.
message_id
string
The ID of the message that the File is attached to.
OBJECT The message file object (v1)

{
  "id": "file-abc123",
  "object": "thread.message.file",
  "created_at": 1698107661,
  "message_id": "message_QLoItBbqwyAJEzlTy4y9kOMM",
  "file_id": "file-abc123"
}
Runs (v1)
Legacy
Represents an execution run on a thread.

Related guide: Assistants
Create run (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/runs
Create a run.

Path parameters

thread_id
string
Required
The ID of the thread to run.
Request body

assistant_id
string
Required
The ID of the assistant to use to execute this run.
model
string
Optional
The ID of the Model to be used to execute this run. If a value is provided here, it will override the model associated with the assistant. If not, the model associated with the assistant will be used.
instructions
string or null
Optional
Overrides the instructions of the assistant. This is useful for modifying the behavior on a per-run basis.
additional_instructions
string or null
Optional
Appends additional instructions at the end of the instructions for the run. This is useful for modifying the behavior on a per-run basis without overriding other instructions.
additional_messages
array or null
Optional
Adds additional messages to the thread before creating the run.

Show properties
tools
array or null
Optional
Override the tools the assistant can use for this run. This is useful for modifying the behavior on a per-run basis.

Show possible types
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
temperature
number or null
Optional
Defaults to 1
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
top_p
number or null
Optional
Defaults to 1
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
stream
boolean or null
Optional
If true, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a data: [DONE] message.
max_prompt_tokens
integer or null
Optional
The maximum number of prompt tokens that may be used over the course of the run. The run will make a best effort to use only the number of prompt tokens specified, across multiple turns of the run. If the run exceeds the number of prompt tokens specified, the run will end with status complete. See incomplete_details for more info.
max_completion_tokens
integer or null
Optional
The maximum number of completion tokens that may be used over the course of the run. The run will make a best effort to use only the number of completion tokens specified, across multiple turns of the run. If the run exceeds the number of completion tokens specified, the run will end with status complete. See incomplete_details for more info.
truncation_strategy
object
Optional

Show properties
tool_choice
string or object
Optional
Controls which (if any) tool is called by the model. none means the model will not call any tools and instead generates a message. auto is the default value and means the model can pick between generating a message or calling a tool. Specifying a particular tool like {"type": "TOOL_TYPE"} or {"type": "function", "function": {"name": "my_function"}} forces the model to call that tool.

Show possible types
response_format
string or object
Optional
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
Returns

A run object.


Default

Streaming

Streaming with Functions
Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.runs.create(
  thread_id="thread_abc123",
  assistant_id="asst_abc123"
)

print(run)
Response

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1699063290,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "queued",
  "started_at": 1699063290,
  "expires_at": null,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": 1699063291,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": null,
  "incomplete_details": null,
  "tools": [
    {
      "type": "code_interpreter"
    }
  ],
  "file_ids": [
    "file-abc123",
    "file-abc456"
  ],
  "metadata": {},
  "usage": null,
  "temperature": 1.0,
  "top_p": 1.0,
  "max_prompt_tokens": 1000,
  "max_completion_tokens": 1000,
  "truncation_strategy": {
    "type": "auto",
    "last_messages": null
  },
  "response_format": "auto",
  "tool_choice": "auto"
}
Create thread and run (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/runs
Create a thread and run it in one request.

Request body

assistant_id
string
Required
The ID of the assistant to use to execute this run.
thread
object
Optional

Show properties
model
string
Optional
The ID of the Model to be used to execute this run. If a value is provided here, it will override the model associated with the assistant. If not, the model associated with the assistant will be used.
instructions
string or null
Optional
Override the default system message of the assistant. This is useful for modifying the behavior on a per-run basis.
tools
array or null
Optional
Override the tools the assistant can use for this run. This is useful for modifying the behavior on a per-run basis.
metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
temperature
number or null
Optional
Defaults to 1
What sampling temperature to use, between 0 and 2. Higher values like 0.8 will make the output more random, while lower values like 0.2 will make it more focused and deterministic.
top_p
number or null
Optional
Defaults to 1
An alternative to sampling with temperature, called nucleus sampling, where the model considers the results of the tokens with top_p probability mass. So 0.1 means only the tokens comprising the top 10% probability mass are considered.

We generally recommend altering this or temperature but not both.
stream
boolean or null
Optional
If true, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a data: [DONE] message.
max_prompt_tokens
integer or null
Optional
The maximum number of prompt tokens that may be used over the course of the run. The run will make a best effort to use only the number of prompt tokens specified, across multiple turns of the run. If the run exceeds the number of prompt tokens specified, the run will end with status complete. See incomplete_details for more info.
max_completion_tokens
integer or null
Optional
The maximum number of completion tokens that may be used over the course of the run. The run will make a best effort to use only the number of completion tokens specified, across multiple turns of the run. If the run exceeds the number of completion tokens specified, the run will end with status complete. See incomplete_details for more info.
truncation_strategy
object
Optional

Show properties
tool_choice
string or object
Optional
Controls which (if any) tool is called by the model. none means the model will not call any tools and instead generates a message. auto is the default value and means the model can pick between generating a message or calling a tool. Specifying a particular tool like {"type": "TOOL_TYPE"} or {"type": "function", "function": {"name": "my_function"}} forces the model to call that tool.

Show possible types
response_format
string or object
Optional
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
Returns

A run object.


Default

Streaming

Streaming with Functions
Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.create_and_run(
  assistant_id="asst_abc123",
  thread={
    "messages": [
      {"role": "user", "content": "Explain deep learning to a 5 year old."}
    ]
  }
)

print(run)
Response

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1699076792,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "queued",
  "started_at": null,
  "expires_at": 1699077392,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": null,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": "You are a helpful assistant.",
  "tools": [],
  "file_ids": [],
  "metadata": {},
  "usage": null,
  "temperature": 1
}
List runs (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/runs
Returns a list of runs belonging to a thread.

Path parameters

thread_id
string
Required
The ID of the thread the run belongs to.
Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
Returns

A list of run objects.

Example request

from openai import OpenAI
client = OpenAI()

runs = client.beta.threads.runs.list(
  "thread_abc123"
)

print(runs)
Response

{
  "object": "list",
  "data": [
    {
      "id": "run_abc123",
      "object": "thread.run",
      "created_at": 1699075072,
      "assistant_id": "asst_abc123",
      "thread_id": "thread_abc123",
      "status": "completed",
      "started_at": 1699075072,
      "expires_at": null,
      "cancelled_at": null,
      "failed_at": null,
      "completed_at": 1699075073,
      "last_error": null,
      "model": "gpt-4-turbo",
      "instructions": null,
      "incomplete_details": null,
      "tools": [
        {
          "type": "code_interpreter"
        }
      ],
      "file_ids": [
        "file-abc123",
        "file-abc456"
      ],
      "metadata": {},
      "usage": {
        "prompt_tokens": 123,
        "completion_tokens": 456,
        "total_tokens": 579
      },
      "temperature": 1.0,
      "top_p": 1.0,
      "max_prompt_tokens": 1000,
      "max_completion_tokens": 1000,
      "truncation_strategy": {
        "type": "auto",
        "last_messages": null
      },
      "response_format": "auto",
      "tool_choice": "auto"
    },
    {
      "id": "run_abc456",
      "object": "thread.run",
      "created_at": 1699063290,
      "assistant_id": "asst_abc123",
      "thread_id": "thread_abc123",
      "status": "completed",
      "started_at": 1699063290,
      "expires_at": null,
      "cancelled_at": null,
      "failed_at": null,
      "completed_at": 1699063291,
      "last_error": null,
      "model": "gpt-4-turbo",
      "instructions": null,
      "incomplete_details": null,
      "tools": [
        {
          "type": "code_interpreter"
        }
      ],
      "file_ids": [
        "file-abc123",
        "file-abc456"
      ],
      "metadata": {},
      "usage": {
        "prompt_tokens": 123,
        "completion_tokens": 456,
        "total_tokens": 579
      },
      "temperature": 1.0,
      "top_p": 1.0,
      "max_prompt_tokens": 1000,
      "max_completion_tokens": 1000,
      "truncation_strategy": {
        "type": "auto",
        "last_messages": null
      },
      "response_format": "auto",
      "tool_choice": "auto"
    }
  ],
  "first_id": "run_abc123",
  "last_id": "run_abc456",
  "has_more": false
}
List run steps (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}/steps
Returns a list of run steps belonging to a run.

Path parameters

thread_id
string
Required
The ID of the thread the run and run steps belong to.
run_id
string
Required
The ID of the run the run steps belong to.
Query parameters

limit
integer
Optional
Defaults to 20
A limit on the number of objects to be returned. Limit can range between 1 and 100, and the default is 20.
order
string
Optional
Defaults to desc
Sort order by the created_at timestamp of the objects. asc for ascending order and desc for descending order.
after
string
Optional
A cursor for use in pagination. after is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include after=obj_foo in order to fetch the next page of the list.
before
string
Optional
A cursor for use in pagination. before is an object ID that defines your place in the list. For instance, if you make a list request and receive 100 objects, ending with obj_foo, your subsequent call can include before=obj_foo in order to fetch the previous page of the list.
Returns

A list of run step objects.

Example request

from openai import OpenAI
client = OpenAI()

run_steps = client.beta.threads.runs.steps.list(
    thread_id="thread_abc123",
    run_id="run_abc123"
)

print(run_steps)
Response

{
  "object": "list",
  "data": [
    {
      "id": "step_abc123",
      "object": "thread.run.step",
      "created_at": 1699063291,
      "run_id": "run_abc123",
      "assistant_id": "asst_abc123",
      "thread_id": "thread_abc123",
      "type": "message_creation",
      "status": "completed",
      "cancelled_at": null,
      "completed_at": 1699063291,
      "expired_at": null,
      "failed_at": null,
      "last_error": null,
      "step_details": {
        "type": "message_creation",
        "message_creation": {
          "message_id": "msg_abc123"
        }
      },
      "usage": {
        "prompt_tokens": 123,
        "completion_tokens": 456,
        "total_tokens": 579
      }
    }
  ],
  "first_id": "step_abc123",
  "last_id": "step_abc456",
  "has_more": false
}
Retrieve run (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}
Retrieves a run.

Path parameters

thread_id
string
Required
The ID of the thread that was run.
run_id
string
Required
The ID of the run to retrieve.
Returns

The run object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.runs.retrieve(
  thread_id="thread_abc123",
  run_id="run_abc123"
)

print(run)
Response

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1699075072,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "completed",
  "started_at": 1699075072,
  "expires_at": null,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": 1699075073,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": null,
  "incomplete_details": null,
  "tools": [
    {
      "type": "code_interpreter"
    }
  ],
  "file_ids": [
    "file-abc123",
    "file-abc456"
  ],
  "metadata": {},
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  },
  "temperature": 1.0,
  "top_p": 1.0,
  "max_prompt_tokens": 1000,
  "max_completion_tokens": 1000,
  "truncation_strategy": {
    "type": "auto",
    "last_messages": null
  },
  "response_format": "auto",
  "tool_choice": "auto"
}
Retrieve run step (v1)
Legacy
GET
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}/steps/{step_id}
Retrieves a run step.

Path parameters

thread_id
string
Required
The ID of the thread to which the run and run step belongs.
run_id
string
Required
The ID of the run to which the run step belongs.
step_id
string
Required
The ID of the run step to retrieve.
Returns

The run step object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

run_step = client.beta.threads.runs.steps.retrieve(
    thread_id="thread_abc123",
    run_id="run_abc123",
    step_id="step_abc123"
)

print(run_step)
Response

{
  "id": "step_abc123",
  "object": "thread.run.step",
  "created_at": 1699063291,
  "run_id": "run_abc123",
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "type": "message_creation",
  "status": "completed",
  "cancelled_at": null,
  "completed_at": 1699063291,
  "expired_at": null,
  "failed_at": null,
  "last_error": null,
  "step_details": {
    "type": "message_creation",
    "message_creation": {
      "message_id": "msg_abc123"
    }
  },
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  }
}
Modify run (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}
Modifies a run.

Path parameters

thread_id
string
Required
The ID of the thread that was run.
run_id
string
Required
The ID of the run to modify.
Request body

metadata
map
Optional
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
Returns

The modified run object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.runs.update(
  thread_id="thread_abc123",
  run_id="run_abc123",
  metadata={"user_id": "user_abc123"},
)

print(run)
Response

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1699075072,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "completed",
  "started_at": 1699075072,
  "expires_at": null,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": 1699075073,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": null,
  "incomplete_details": null,
  "tools": [
    {
      "type": "code_interpreter"
    }
  ],
  "file_ids": [
    "file-abc123",
    "file-abc456"
  ],
  "metadata": {
    "user_id": "user_abc123"
  },
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  },
  "temperature": 1.0,
  "top_p": 1.0,
  "max_prompt_tokens": 1000,
  "max_completion_tokens": 1000,
  "truncation_strategy": {
    "type": "auto",
    "last_messages": null
  },
  "response_format": "auto",
  "tool_choice": "auto"
}
Submit tool outputs to run (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}/submit_tool_outputs
When a run has the status: "requires_action" and required_action.type is submit_tool_outputs, this endpoint can be used to submit the outputs from the tool calls once they're all completed. All outputs must be submitted in a single request.

Path parameters

thread_id
string
Required
The ID of the thread to which this run belongs.
run_id
string
Required
The ID of the run that requires the tool output submission.
Request body

tool_outputs
array
Required
A list of tools for which the outputs are being submitted.

Show properties
stream
boolean or null
Optional
If true, returns a stream of events that happen during the Run as server-sent events, terminating when the Run enters a terminal state with a data: [DONE] message.
Returns

The modified run object matching the specified ID.


Default

Streaming
Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.runs.submit_tool_outputs(
  thread_id="thread_123",
  run_id="run_123",
  tool_outputs=[
    {
      "tool_call_id": "call_001",
      "output": "70 degrees and sunny."
    }
  ]
)

print(run)
Response

{
  "id": "run_123",
  "object": "thread.run",
  "created_at": 1699075592,
  "assistant_id": "asst_123",
  "thread_id": "thread_123",
  "status": "queued",
  "started_at": 1699075592,
  "expires_at": 1699076192,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": null,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": null,
  "incomplete_details": null,
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_current_weather",
        "description": "Get the current weather in a given location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The city and state, e.g. San Francisco, CA"
            },
            "unit": {
              "type": "string",
              "enum": ["celsius", "fahrenheit"]
            }
          },
          "required": ["location"]
        }
      }
    }
  ],
  "file_ids": [],
  "metadata": {},
  "usage": null,
  "temperature": 1.0,
  "top_p": 1.0,
  "max_prompt_tokens": 1000,
  "max_completion_tokens": 1000,
  "truncation_strategy": {
    "type": "auto",
    "last_messages": null
  },
  "response_format": "auto",
  "tool_choice": "auto"
}
Cancel a run (v1)
Legacy
POST
 
https://api.openai.com/v1/threads/{thread_id}/runs/{run_id}/cancel
Cancels a run that is in_progress.

Path parameters

thread_id
string
Required
The ID of the thread to which this run belongs.
run_id
string
Required
The ID of the run to cancel.
Returns

The modified run object matching the specified ID.

Example request

from openai import OpenAI
client = OpenAI()

run = client.beta.threads.runs.cancel(
  thread_id="thread_abc123",
  run_id="run_abc123"
)

print(run)
Response

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1699076126,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "cancelling",
  "started_at": 1699076126,
  "expires_at": 1699076726,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": null,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": "You summarize books.",
  "tools": [
    {
      "type": "retrieval"
    }
  ],
  "file_ids": [],
  "metadata": {},
  "usage": null,
  "temperature": 1.0,
  "top_p": 1.0,
}
The run object (v1)
Legacy
Represents an execution run on a thread.

id
string
The identifier, which can be referenced in API endpoints.
object
string
The object type, which is always thread.run.
created_at
integer
The Unix timestamp (in seconds) for when the run was created.
thread_id
string
The ID of the thread that was executed on as a part of this run.
assistant_id
string
The ID of the assistant used for execution of this run.
status
string
The status of the run, which can be either queued, in_progress, requires_action, cancelling, cancelled, failed, completed, or expired.
required_action
object or null
Details on the action required to continue the run. Will be null if no action is required.

Show properties
last_error
object or null
The last error associated with this run. Will be null if there are no errors.

Show properties
expires_at
integer or null
The Unix timestamp (in seconds) for when the run will expire.
started_at
integer or null
The Unix timestamp (in seconds) for when the run was started.
cancelled_at
integer or null
The Unix timestamp (in seconds) for when the run was cancelled.
failed_at
integer or null
The Unix timestamp (in seconds) for when the run failed.
completed_at
integer or null
The Unix timestamp (in seconds) for when the run was completed.
incomplete_details
object or null
Details on why the run is incomplete. Will be null if the run is not incomplete.

Show properties
model
string
The model that the assistant used for this run.
instructions
string
The instructions that the assistant used for this run.
tools
array
The list of tools that the assistant used for this run.

Show possible types
file_ids
array
The list of File IDs the assistant used for this run.
metadata
map
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
usage
temperature
number or null
The sampling temperature used for this run. If not set, defaults to 1.
top_p
number or null
The nucleus sampling value used for this run. If not set, defaults to 1.
max_prompt_tokens
integer or null
The maximum number of prompt tokens specified to have been used over the course of the run.
max_completion_tokens
integer or null
The maximum number of completion tokens specified to have been used over the course of the run.
truncation_strategy
object

Show properties
tool_choice
string or object
Controls which (if any) tool is called by the model. none means the model will not call any tools and instead generates a message. auto is the default value and means the model can pick between generating a message or calling a tool. Specifying a particular tool like {"type": "TOOL_TYPE"} or {"type": "function", "function": {"name": "my_function"}} forces the model to call that tool.

Show possible types
response_format
string or object
Specifies the format that the model must output. Compatible with GPT-4o, GPT-4 Turbo, and all GPT-3.5 Turbo models since gpt-3.5-turbo-1106.

Setting to { "type": "json_object" } enables JSON mode, which guarantees the message the model generates is valid JSON.

Important: when using JSON mode, you must also instruct the model to produce JSON yourself via a system or user message. Without this, the model may generate an unending stream of whitespace until the generation reaches the token limit, resulting in a long-running and seemingly "stuck" request. Also note that the message content may be partially cut off if finish_reason="length", which indicates the generation exceeded max_tokens or the conversation exceeded the max context length.

Show possible types
OBJECT The run object (v1)

{
  "id": "run_abc123",
  "object": "thread.run",
  "created_at": 1698107661,
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "status": "completed",
  "started_at": 1699073476,
  "expires_at": null,
  "cancelled_at": null,
  "failed_at": null,
  "completed_at": 1699073498,
  "last_error": null,
  "model": "gpt-4-turbo",
  "instructions": null,
  "tools": [{"type": "retrieval"}, {"type": "code_interpreter"}],
  "file_ids": [],
  "metadata": {},
  "incomplete_details": null,
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  },
  "temperature": 1.0,
  "top_p": 1.0,
  "max_prompt_tokens": 1000,
  "max_completion_tokens": 1000,
  "truncation_strategy": {
    "type": "auto",
    "last_messages": null
  },
  "response_format": "auto",
  "tool_choice": "auto"
}
The run step object (v1)
Legacy
Represents a step in execution of a run.

id
string
The identifier of the run step, which can be referenced in API endpoints.
object
string
The object type, which is always thread.run.step.
created_at
integer
The Unix timestamp (in seconds) for when the run step was created.
assistant_id
string
The ID of the assistant associated with the run step.
thread_id
string
The ID of the thread that was run.
run_id
string
The ID of the run that this run step is a part of.
type
string
The type of run step, which can be either message_creation or tool_calls.
status
string
The status of the run step, which can be either in_progress, cancelled, failed, completed, or expired.
step_details
object
The details of the run step.

Show possible types
last_error
object or null
The last error associated with this run step. Will be null if there are no errors.

Show properties
expired_at
integer or null
The Unix timestamp (in seconds) for when the run step expired. A step is considered expired if the parent run is expired.
cancelled_at
integer or null
The Unix timestamp (in seconds) for when the run step was cancelled.
failed_at
integer or null
The Unix timestamp (in seconds) for when the run step failed.
completed_at
integer or null
The Unix timestamp (in seconds) for when the run step completed.
metadata
map
Set of 16 key-value pairs that can be attached to an object. This can be useful for storing additional information about the object in a structured format. Keys can be a maximum of 64 characters long and values can be a maxium of 512 characters long.
usage
OBJECT The run step object (v1)

{
  "id": "step_abc123",
  "object": "thread.run.step",
  "created_at": 1699063291,
  "run_id": "run_abc123",
  "assistant_id": "asst_abc123",
  "thread_id": "thread_abc123",
  "type": "message_creation",
  "status": "completed",
  "cancelled_at": null,
  "completed_at": 1699063291,
  "expired_at": null,
  "failed_at": null,
  "last_error": null,
  "step_details": {
    "type": "message_creation",
    "message_creation": {
      "message_id": "msg_abc123"
    }
  },
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  }
}
Streaming (v1)
Legacy
Stream the result of executing a Run or resuming a Run after submitting tool outputs.

You can stream events from the Create Thread and Run, Create Run, and Submit Tool Outputs endpoints by passing "stream": true. The response will be a Server-Sent events stream.

Our Node and Python SDKs provide helpful utilities to make streaming easy. Reference the Assistants API quickstart to learn more.
The message delta object (v1)
Legacy
Represents a message delta i.e. any changed fields on a message during streaming.

id
string
The identifier of the message, which can be referenced in API endpoints.
object
string
The object type, which is always thread.message.delta.
delta
object
The delta containing the fields that have changed on the Message.

Show properties
OBJECT The message delta object (v1)

{
  "id": "msg_123",
  "object": "thread.message.delta",
  "delta": {
    "content": [
      {
        "index": 0,
        "type": "text",
        "text": { "value": "Hello", "annotations": [] }
      }
    ]
  }
}
The run step delta object (v1)
Legacy
Represents a run step delta i.e. any changed fields on a run step during streaming.

id
string
The identifier of the run step, which can be referenced in API endpoints.
object
string
The object type, which is always thread.run.step.delta.
delta
object
The delta containing the fields that have changed on the run step.

Show properties
OBJECT The run step delta object (v1)

{
  "id": "step_123",
  "object": "thread.run.step.delta",
  "delta": {
    "step_details": {
      "type": "tool_calls",
      "tool_calls": [
        {
          "index": 0,
          "id": "call_123",
          "type": "code_interpreter",
          "code_interpreter": { "input": "", "outputs": [] }
        }
      ]
    }
  }
}
Assistant stream events (v1)
Legacy
Represents an event emitted when streaming a Run.

Each event in a server-sent events stream has an event and data property:


event: thread.created
data: {"id": "thread_123", "object": "thread", ...}
We emit events whenever a new object is created, transitions to a new state, or is being streamed in parts (deltas). For example, we emit thread.run.created when a new run is created, thread.run.completed when a run completes, and so on. When an Assistant chooses to create a message during a run, we emit a thread.message.created event, a thread.message.in_progress event, many thread.message.delta events, and finally a thread.message.completed event.

We may add additional events over time, so we recommend handling unknown events gracefully in your code. See the Assistants API quickstart to learn how to integrate the Assistants API with streaming.

thread.created
data is a thread
Occurs when a new thread is created.
thread.run.created
data is a run
Occurs when a new run is created.
thread.run.queued
data is a run
Occurs when a run moves to a queued status.
thread.run.in_progress
data is a run
Occurs when a run moves to an in_progress status.
thread.run.requires_action
data is a run
Occurs when a run moves to a requires_action status.
thread.run.completed
data is a run
Occurs when a run is completed.
thread.run.failed
data is a run
Occurs when a run fails.
thread.run.cancelling
data is a run
Occurs when a run moves to a cancelling status.
thread.run.cancelled
data is a run
Occurs when a run is cancelled.
thread.run.expired
data is a run
Occurs when a run expires.
thread.run.step.created
data is a run step
Occurs when a run step is created.
thread.run.step.in_progress
data is a run step
Occurs when a run step moves to an in_progress state.
thread.run.step.delta
data is a run step delta
Occurs when parts of a run step are being streamed.
thread.run.step.completed
data is a run step
Occurs when a run step is completed.
thread.run.step.failed
data is a run step
Occurs when a run step fails.
thread.run.step.cancelled
data is a run step
Occurs when a run step is cancelled.
thread.run.step.expired
data is a run step
Occurs when a run step expires.
thread.message.created
data is a message
Occurs when a message is created.
thread.message.in_progress
data is a message
Occurs when a message moves to an in_progress state.
thread.message.delta
data is a message delta
Occurs when parts of a Message are being streamed.
thread.message.completed
data is a message
Occurs when a message is completed.
thread.message.incomplete
data is a message
Occurs when a message ends before it is completed.
error
data is an error
Occurs when an error occurs. This can happen due to an internal server error or a timeout.
done
data is [DONE]
Occurs when a stream ends.