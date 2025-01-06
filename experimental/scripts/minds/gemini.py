from abc import ABC, abstractmethod
from dataclasses import dataclass
from typing import List, Dict, Any, Optional, TypeVar, Generic, Union
import json
import aiohttp
from datetime import datetime
import uuid
import asyncio
from enum import Enum

# Response types and helpers
class ResponseType(str, Enum):
    TEXT = "text"
    TOOL_CALL = "tool_call"
    UNKNOWN = "unknown"

@dataclass
class Content:
    parts: List[Any]
    role: str

@dataclass
class GenerateContentResponse:
    candidates: List['ResponseCandidate']

@dataclass
class ResponseCandidate:
    content: Content

@dataclass
class Text:
    value: str
    
    def __str__(self):
        return self.value

@dataclass
class FunctionCall:
    name: str
    args: Dict[str, Any]

@dataclass
class FunctionResponse:
    name: str
    response: Dict[str, Any]

@dataclass
class ToolCall:
    function: Optional['FunctionCall'] = None
    type: Optional[str] = None
    id: Optional[str] = None

@dataclass
class Schema:
    type: str
    description: Optional[str] = None
    properties: Optional[Dict[str, 'Schema']] = None
    items: Optional['Schema'] = None
    required: Optional[List[str]] = None
    enum: Optional[List[Any]] = None
    nullable: bool = False

class Response:
    def __init__(self, raw: GenerateContentResponse):
        if not raw.candidates:
            raise ValueError("No candidates in Gemini response")
        if not raw.candidates[0].content:
            raise ValueError("Candidate content is nil")
        if not raw.candidates[0].content.parts:
            raise ValueError("Candidate content has no parts")
        self.raw = raw

    def to_messages(self) -> List['Message']:
        if self.type() == ResponseType.TEXT:
            text, _ = self.text()
            return [Message(
                role=self.raw.candidates[0].content.role,
                content=text
            )]
        elif self.type() == ResponseType.TOOL_CALL:
            calls, _ = self.tool_calls()
            return new_tool_call_response(calls).to_messages()
        else:
            raise ValueError("Unknown response type")

    def __str__(self) -> str:
        response_type = self.type()
        if response_type == ResponseType.TEXT:
            text, _ = self.text()
            return text
        elif response_type == ResponseType.TOOL_CALL:
            calls, _ = self.tool_calls()
            return f"function call: {calls}"
        return "Unknown response type"

    def type(self) -> ResponseType:
        if not self.raw.candidates:
            return ResponseType.UNKNOWN
        
        candidate = self.raw.candidates[0]
        if not candidate.content.parts:
            return ResponseType.UNKNOWN

        part = candidate.content.parts[0]
        if isinstance(part, Text):
            return ResponseType.TEXT
        elif isinstance(part, FunctionCall):
            return ResponseType.TOOL_CALL
        return ResponseType.UNKNOWN

    def text(self) -> tuple[str, bool]:
        if self.type() != ResponseType.TEXT:
            return "", False
        
        text = self.raw.candidates[0].content.parts[0]
        if not isinstance(text, Text):
            return "", False
        
        return str(text), True

    def tool_calls(self) -> tuple[List[ToolCall], bool]:
        if self.type() != ResponseType.TOOL_CALL:
            return [], False

        func_call = self.raw.candidates[0].content.parts[0]
        if not isinstance(func_call, FunctionCall):
            return [], False

        return [ToolCall(
            function=FunctionCall(
                name=func_call.name,
                arguments=json.dumps(func_call.args).encode()
            )
        )], True

    def raw_response(self) -> GenerateContentResponse:
        return self.raw

class ChatSession:
    def __init__(self, model: 'GenerativeModel'):
        self.model = model
        self.history: List[Content] = []

    async def send_message(self, ctx: dict, *parts: Union[Text, FunctionCall]) -> GenerateContentResponse:
        return await self.model.generate_content(ctx, *parts)

class GenerativeModel(ABC):
    @abstractmethod
    async def generate_content(self, ctx: dict, *parts: Union[Text, FunctionCall]) -> GenerateContentResponse:
        pass

    @abstractmethod
    async def count_tokens(self, ctx: dict, *parts: Union[Text, FunctionCall]) -> int:
        pass

    def start_chat(self) -> ChatSession:
        return ChatSession(self)

class Provider:
    def __init__(self, client: 'Client'):
        self.client = client
        self.model = None

    async def initialize(self):
        self.model = await self.client.generative_model()

    def close(self):
        self.client.close()

    def model_name(self) -> str:
        return self.client.options().model_name

    async def generate_content(self, ctx: dict, req: 'Request') -> Response:
        if ctx.get('done'):
            raise asyncio.CancelledError()

        chat_session = self.model.start_chat()

        system_prompt = None
        history: List[Content] = []

        if req.options.get('response_schema'):
            if hasattr(self.model, 'response_mime_type'):
                schema = convert_schema(req.options['response_schema']['definition'])
                self.model.response_mime_type = "application/json"
                self.model.response_schema = schema

        for msg in req.messages:
            if msg.role == "system":
                if not system_prompt:
                    system_prompt = Content(parts=[], role="system")
                system_prompt.parts.append(Text(msg.content))
            elif msg.role == "function":
                response = json.loads("test")  # Mock response for example
                history.append(Content(
                    parts=[FunctionResponse(
                        name=msg.name,
                        response=response
                    )],
                    role=msg.role
                ))
            else:
                if not msg.role:
                    msg.role = "user"
                if msg.role == "assistant":
                    msg.role = "model"
                history.append(Content(
                    parts=[Text(msg.content)],
                    role=msg.role
                ))

        if system_prompt and hasattr(self.model, 'system_instruction'):
            self.model.system_instruction = system_prompt

        prompt = history[-1].parts
        history = history[:-1]
        chat_session.history = history

        raw = await chat_session.send_message(ctx, *prompt)
        resp = Response(raw)

        return await handle_function_calls(ctx, resp, self.client.options().registry)

    async def handle_thread(self, ctx: dict, tc: 'ThreadContext', next_handler: 'ThreadHandler') -> None:
        req = Request(messages=tc.thread().messages)
        
        resp = await self.generate_content(ctx, req)
        
        tc.append(Message(
            role="assistant",
            content=str(resp)
        ))

        await next_handler.handle_thread(ctx, tc, next_handler)

def new_provider(client: 'Client') -> Provider:
    provider = Provider(client)
    return provider

# Helper functions
def convert_schema(definition: Dict[str, Any]) -> Schema:
    schema = Schema(type=definition['type'])
    
    if 'description' in definition:
        schema.description = definition['description']
    
    if 'properties' in definition:
        schema.properties = {
            k: convert_schema(v) for k, v in definition['properties'].items()
        }
    
    if 'items' in definition:
        schema.items = convert_schema(definition['items'])
    
    if 'required' in definition:
        schema.required = definition['required']
    
    if 'enum' in definition:
        schema.enum = definition['enum']
    
    return schema