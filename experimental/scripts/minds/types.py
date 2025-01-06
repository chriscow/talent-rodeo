from abc import ABC, abstractmethod
import json
import dataclasses
from typing import List, Dict, Any, Optional
from datetime import datetime
import uuid
import asyncio
from dataclasses import dataclass
from typing import Callable, TypeVar, Generic

# Base interfaces and types
class ContentGenerator(ABC):
    @abstractmethod
    def model_name(self) -> str:
        pass
    
    @abstractmethod
    async def generate_content(self, ctx: dict, request: 'Request') -> 'Response':
        pass
    
    @abstractmethod
    def close(self) -> None:
        pass

class Embedder(ABC):
    @abstractmethod
    async def create_embeddings(self, model: str, input: List[str]) -> List[List[float]]:
        pass

class KVStore(ABC):
    @abstractmethod
    async def save(self, ctx: dict, key: bytes, value: bytes) -> None:
        pass
    
    @abstractmethod
    async def load(self, ctx: dict, key: bytes) -> bytes:
        pass

class TokenCounter(ABC):
    @abstractmethod
    def count_tokens(self, text: str) -> int:
        pass

# Message thread related classes
@dataclass
class Message:
    role: str
    content: str
    name: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None
    tool_call_id: Optional[str] = None
    tool_calls: Optional[List[dict]] = None

@dataclass
class MessageThread:
    uuid: str
    metadata: Dict[str, Any]
    messages: List[Message]

    @classmethod
    def new(cls, metadata: Dict[str, Any], messages: List[Message]) -> 'MessageThread':
        return cls(
            uuid=str(uuid.uuid4()),
            metadata=metadata or {},
            messages=messages or []
        )
    
    def last_message(self) -> Optional[Message]:
        return self.messages[-1] if self.messages else None
    
    def exclude(self, roles: List[str]) -> List[Message]:
        return [msg for msg in self.messages if msg.role not in roles]
    
    def only(self, roles: List[str]) -> List[Message]:
        return [msg for msg in self.messages if msg.role in roles]
    
    def token_count(self, tokenizer: TokenCounter) -> int:
        return sum(tokenizer.count_tokens(msg.content) for msg in self.messages)

@dataclass
class ThreadContext:
    options: Dict[str, Any]
    meta: Dict[str, Any]
    mt: MessageThread
    
    @classmethod
    def new(cls, meta: Dict[str, Any], mt: MessageThread, opts: Optional[Dict[str, Any]] = None) -> 'ThreadContext':
        return cls(
            options=opts or {},
            meta=meta or {},
            mt=mt
        )
    
    def to_json(self) -> str:
        return json.dumps({
            "options": self.options,
            "meta": self.meta,
            "thread": {
                "uuid": self.mt.uuid,
                "metadata": self.mt.metadata,
                "messages": [dataclasses.asdict(msg) for msg in self.mt.messages]
            }
        })
    
    @classmethod
    def from_json(cls, data: str) -> 'ThreadContext':
        parsed = json.loads(data)
        return cls(
            options=parsed["options"],
            meta=parsed["meta"],
            mt=MessageThread(
                uuid=parsed["thread"]["uuid"],
                metadata=parsed["thread"]["metadata"],
                messages=[Message(**msg) for msg in parsed["thread"]["messages"]]
            )
        )
    
    def thread(self) -> MessageThread:
        messages = self.mt.messages.copy()
        return MessageThread(
            uuid=self.mt.uuid,
            metadata=self.meta.copy(),
            messages=messages
        )
    
    def append(self, *messages: Message) -> None:
        self.mt.messages.extend(messages)
    
    def metadata(self) -> Dict[str, Any]:
        return self.meta.copy()
    
    def add_metadata(self, key: str, value: Any) -> None:
        self.mt.metadata[key] = value

# Thread handler interface and implementation
class ThreadHandler(ABC):
    @abstractmethod
    async def handle_thread(self, ctx: dict, thread: ThreadContext, next_handler: 'ThreadHandler') -> None:
        pass

class ThreadHandlerFunc:
    def __init__(self, func: Callable):
        self.func = func
    
    async def handle_thread(self, ctx: dict, thread: ThreadContext, next_handler: ThreadHandler) -> None:
        return await self.func(ctx, thread, next_handler)

class NoopPipelineHandler(ThreadHandler):
    async def handle_thread(self, ctx: dict, thread: ThreadContext, next_handler: ThreadHandler) -> None:
        pass

def adapt_thread_handler(th: ThreadHandler):
    async def handler(msg: dict) -> List[dict]:
        tc = ThreadContext.from_json(json.dumps(msg["payload"]))
        ctx = {}  # or extract from msg if you have a context
        
        await th.handle_thread(ctx, tc, NoopPipelineHandler())
        
        payload = json.loads(tc.to_json())
        return [{"uuid": str(uuid.uuid4()), "payload": payload}]
    
    return handler

# Function call handling
@dataclass
class FunctionCall:
    name: str
    arguments: bytes
    result: Optional[bytes] = None

@dataclass
class ToolCall:
    id: Optional[str] = None
    type: Optional[str] = None
    function: Optional[FunctionCall] = None

class Tool(ABC):
    @abstractmethod
    def type(self) -> str:
        pass
    
    @abstractmethod
    def name(self) -> str:
        pass
    
    @abstractmethod
    def description(self) -> str:
        pass
    
    @abstractmethod
    def parameters(self) -> dict:
        pass
    
    @abstractmethod
    async def call(self, ctx: dict, params: bytes) -> bytes:
        pass

class ToolRegistry:
    def __init__(self):
        self.tools: Dict[str, Tool] = {}
    
    def register(self, tool: Tool) -> None:
        if tool.name() in self.tools:
            raise ValueError(f"Tool {tool.name()} already registered")
        self.tools[tool.name()] = tool
    
    def lookup(self, name: str) -> Optional[Tool]:
        return self.tools.get(name)
    
    def list(self) -> List[Tool]:
        return list(self.tools.values())