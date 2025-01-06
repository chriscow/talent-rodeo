import openai
# from openai.types.completion import ChatCompletion
import json, yaml
from enum import Enum
from dataclasses import dataclass, field, asdict
from jinja2 import Template
from datetime import datetime

from enum import Enum
from pydantic import BaseModel, Field
from typing import List, Dict, Optional

import prompts.template as template

# Conversation Enums
class ConversationStage(str, Enum):
    PLANNING_CONTACT = "planning_contact"
    INITIAL_CONTACT = "initial_contact"
    DISCOVERY = "discovery"
    DEEP_DIVE = "deep_dive"
    FOLLOW_UP = "follow_up"
    CLOSING = "closing"

class EngagementLevel(str, Enum):
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"

class NextResponse(BaseModel):
    response: str

# State Classes
class CandidateInfo(BaseModel):
    known_facts: List[str] = Field(default_factory=list)
    inferred_preferences: List[str] = Field(default_factory=list)
    potential_concerns: List[str] = Field(default_factory=list)
    engagement_level: EngagementLevel = EngagementLevel.MEDIUM
    resume: Dict = Field(default_factory=dict)
    name: str = ""
    phone_number: str = ""

class ConversationContext(BaseModel):
    last_topic: str = ""
    knowledge: List[str] = Field(default_factory=list)
    unaddressed_points: List[str] = Field(default_factory=list)
    follow_up_items: List[str] = Field(default_factory=list)
    next_response_goals: List[str] = Field(default_factory=list)
    next_response_plan: List[str] = Field(default_factory=list)

class ConversationState(BaseModel):
    stage: ConversationStage
    candidate_info: CandidateInfo
    conversation_context: ConversationContext

#
# You can also do this:
#
# class GetDeliveryDate(BaseModel):
#   order_id: str
#
# tools = [openai.pydantic_function_tool(GetDeliveryDate)]
#

FUNCTION_SCHEMAS = [
    {
        "type": "function",
        "function": {
            "name": "call_candidate",
            "description": "Call the candidate on the phone",
            "parameters": {
                "type": "object",
                "properties": {
                    "phone_number": {
                        "type": "string",
                        "description": "The candidate's phone number",
                    }
                },
                "required": ["phone_number"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "email_candidate",
            "description": "Email the candidate",
            "parameters": {
                "type": "object",
                "properties": {
                    "candidate_email_address": {
                        "type": "string",
                        "description": "The candidate's email address",
                    },
                    "subject": {
                        "type": "string",
                        "description": "The email subject",
                    },
                    "body": {
                        "type": "string",
                        "description": "The email body",
                    },
                },
                "required": ["email_address", "subject", "body"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "end_call",
            "description": "End the current call with the candidate",
            "parameters": {},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_stage",
            "description": "Update the conversation stage",
            "parameters": {
                "type": "object",
                "properties": {
                    "new_stage": {
                        "type": "string",
                        "enum": [e.value for e in ConversationStage],
                    }
                },
                "required": ["new_stage"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "add_known_fact",
            "description": "Add a new discovered fact about the candidate",
            "parameters": {
                "type": "object",
                "properties": {
                    "fact": {
                        "type": "string",
                        "description": "The new fact learned about the candidate",
                    }
                },
                "required": ["fact"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_engagement_level",
            "description": "Update the engagement level of the candidate",
            "parameters": {
                "type": "object",
                "properties": {
                    "level": {
                        "type": "string",
                        "enum": [e.value for e in EngagementLevel],
                    }
                },
                "required": ["level"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "add_follow_up_item",
            "description": "Add a follow-up item to remember for later",
            "parameters": {
                "type": "object",
                "properties": {
                    "item": {
                        "type": "string",
                        "description": "The follow-up item to remember",
                    }
                },
                "required": ["item"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "response_text",
            "description": "Say this response to the candidate",
            "parameters": {
                "type": "object",
                "properties": {
                    "response": {
                        "type": "string",
                        "description": "The response text to say",
                    }
                },
                "required": ["response"],
            },
        },
    },
]

initial_tool_response = {
    "text-content": "(425) 829-1234",
    "resume": {
        "basics": {
            "name": "Alex Chen",
            "label": "Software Engineer",
            "location": "San Francisco, CA",
            "email": "alex.chen@email.com",
            "phone": "(415) 555-0123",
            "profiles": [
                {
                    "network": "LinkedIn",
                    "url": "linkedin.com/in/alexchen",
                }
            ],
        },
        "work": [
            {
                "company": "TechCorp Inc.",
                "position": "Senior Software Engineer",
                "startDate": "2021-01",
                "endDate": "Present",
                "highlights": [
                    "Led development of microservices architecture using Python/FastAPI, reducing API response times by 40%",
                    "Architected and implemented real-time data processing pipeline handling 1M+ events daily using Apache Kafka",
                    "Mentored 3 junior developers and established team code review practices",
                ],
                "technologies": [
                    "Python",
                    "FastAPI",
                    "PostgreSQL",
                    "Redis",
                    "Kafka",
                    "Docker",
                    "AWS",
                ],
            },
            {
                "company": "DataFlow Systems",
                "position": "Software Engineer",
                "startDate": "2018-03",
                "endDate": "2020-12",
                "highlights": [
                    "Developed and maintained Python/Django applications serving 100K+ daily active users",
                    "Implemented automated testing suite reducing bug reports by 30%",
                    "Collaborated with product team to optimize user authentication flow",
                ],
                "technologies": [
                    "Python",
                    "Django",
                    "MySQL",
                    "React",
                    "Jenkins",
                    "GCP",
                ],
            },
            {
                "company": "StartupNow",
                "position": "Junior Software Engineer",
                "startDate": "2016-06",
                "endDate": "2018-02",
                "highlights": [
                    "Built RESTful APIs using Python/Flask for mobile application backend",
                    "Implemented real-time chat features using WebSocket",
                ],
                "technologies": [
                    "Python",
                    "Flask",
                    "MongoDB",
                    "Socket.IO",
                    "Heroku",
                ],
            },
        ],
        "education": [
            {
                "institution": "University of California, Berkeley",
                "area": "Computer Science",
                "studyType": "B.S.",
                "graduationYear": "2016",
            }
        ],
        "skills": {
            "languages": {
                "programming": [
                    {"name": "Python", "years": 7, "level": "Expert"},
                    {
                        "name": "JavaScript",
                        "years": 5,
                        "level": "Advanced",
                    },
                    {"name": "SQL", "level": "Advanced"},
                    {"name": "TypeScript", "level": "Intermediate"},
                ],
                "human": [
                    {"language": "English", "fluency": "Native"},
                    {
                        "language": "Mandarin Chinese",
                        "fluency": "Fluent",
                    },
                ],
            },
            "frameworks": ["Django", "FastAPI", "Flask", "React"],
            "databases": ["PostgreSQL", "MySQL", "MongoDB", "Redis"],
            "cloudDevOps": [
                "AWS",
                "GCP",
                "Docker",
                "Kubernetes",
                "CI/CD",
            ],
            "other": [
                "Microservices Architecture",
                "RESTful APIs",
                "WebSocket",
                "Message Queues",
            ],
        },
        "projects": [
            {
                "name": "Open Source Data Pipeline Framework",
                "highlights": [
                    "Created Python framework for building data pipelines",
                    "500+ GitHub stars, 12 contributors",
                ],
                "technologies": ["Python", "Apache Airflow", "Docker"],
            },
            {
                "name": "Real-time Analytics Dashboard",
                "highlights": [
                    "Built full-stack application for visualizing system metrics"
                ],
                "technologies": [
                    "Python",
                    "FastAPI",
                    "React",
                    "WebSocket",
                    "TimescaleDB",
                ],
            },
        ],
        "certificates": [
            {"name": "AWS Certified Solutions Architect"},
            {"name": "MongoDB Certified Developer"},
        ],
    },
}



# Recruiter Class
class Recruiter:
    client: openai.OpenAI
    system_template: Template
    state: ConversationState
    conversation = []
    function_map = {}
    candidate_response = None

    def __init__(self, openai_api_key: str, system_template_path: str):
        openai.api_key = openai_api_key
        self.client = openai.OpenAI()

        self.initialize_state()

        self.system_template, header = template.from_template(
            system_template_path,
            context={"state": self.state},
        )

        self.function_map = {
            "response_text": self.response_text,
            "update_stage": self.update_stage,
            "add_known_fact": self.add_known_fact,
            "update_engagement_level": self.update_engagement_level,
            "add_follow_up_item": self.add_follow_up_item,
        }
    
    def initialize_state(self) -> ConversationState:
        self.conversation.append(
        {
            "role": "assistant",
            "tool_calls": [
                {
                    "id": "call_12345xyz",
                    "type": "function",
                    "function": {
                        "name": "get_latest_linked_message",
                        "arguments": "",
                    },
                }
            ],
        })

        self.conversation.append(
            {
                "role": "tool",
                "content": json.dumps(initial_tool_response),
                "tool_call_id": "call_12345xyz",
            }
        )
        self.conversation.append(
            {
                "role": "assistant",
                "tool_calls": [
                    {
                        "id": "call_DicWOwAdFk6hWYIvlu1MrZGe",
                        "type": "function",
                        "function": {
                            "name": "call_candidate",
                            "arguments": json.dumps({"phone_number": "(415) 555-0123"}),
                        },
                    }
                ],
            }
        )
        self.conversation.append(
            {
                "role": "tool",
                "content": json.dumps({
                    "call_answered": True,
                    "candidate_response": "Hello?",
                }),
                "tool_call_id": "call_DicWOwAdFk6hWYIvlu1MrZGe",
            }
        )            

        self.state = ConversationState(
            stage=ConversationStage.INITIAL_CONTACT,
            conversation_context=ConversationContext(
                last_topic="",
                knowledge=[
                    "Initial contact via LinkedIn DM",
                    "I Have resume and phone number (below)",
                    "Proactive outreach suggests high engagement",
                    "Need to validate interest level and career goals"
                ],
                next_response_goals=[
                    "Reference their initiative in reaching out",
                    "Acknowledge reviewing their materials",
                    "Create comfortable space for deeper discussion",
                ],
                next_response_plan=[
                    "Warm, professional introduction",
                    "Quick reference to resume review",
                    "Open-ended question about career exploration",
                    "Note tone and engagement for follow-up strategy"
                ],
                unaddressed_points=[], 
                follow_up_items=[],
            ),


            candidate_info=CandidateInfo(
                known_facts=[],
                inferred_preferences=[],
                potential_concerns=[],
                engagement_level=EngagementLevel.MEDIUM,
                resume = initial_tool_response["resume"],
                phone_number=initial_tool_response["text-content"],
            ),
        )

        return self.state


    def call_function_by_name(self, func_name: str, json_args: str):
        # Parse the JSON string into a dictionary
        args = json.loads(json_args)
        
        # Get the function by name
        func = getattr(self, func_name, None)
        
        if func is None:
            raise ValueError(f"Function '{func_name}' not found.")
        
        # Convert string to enum if necessary
        if 'level' in args and isinstance(args['level'], str):
            args['level'] = EngagementLevel(args['level'].lower())
        
        # Call the function with unpacked arguments
        return func(self.state, **args)

    def response_text(self, state: ConversationState, response: str) -> str:
        self.candidate_response = response    
        return "OK"
    

    def update_stage(self, state: ConversationState, new_stage: ConversationStage) -> str:
        state.stage = new_stage
        return state.stage.model_dump_json()


    def add_known_fact(self, state: ConversationState, fact: str) -> str:
        if fact not in state.candidate_info.known_facts:
            state.candidate_info.known_facts.append(fact)
        return state.candidate_info.model_dump_json()


    def update_engagement_level(self, state: ConversationState, level: EngagementLevel) -> str:
        state.candidate_info.engagement_level = level
        return "engagement_level set to " + state.candidate_info.engagement_level


    def add_follow_up_item(self, state: ConversationState, item: str) -> str:
        if item not in state.conversation_context.follow_up_items:
            state.conversation_context.follow_up_items.append(item)
        return state.conversation_context.model_dump_json()

    def call_openai_llm(self, prompt: str, tools: list, tool_choice):
        """Generate a response using GPT-4 with tool requests."""
        self.candidate_response = None
        messages = [{"role": "system", "content":  self.system_template.render({"today":datetime.now().isoformat(), "state":self.state.model_dump_json()})}]
        messages.extend(self.conversation)
        messages.append({"role": "user", "content": prompt})

        completion = self.client.chat.completions.create(
            model="gpt-4o-mini",
            messages=messages,
            tools=tools,
            tool_choice=tool_choice,

        )
        
        return completion
    
    def handle_response(self, prompt: str, llm_response):    
        # Check if the LLM requested tool usage
        state_update = False
        self.conversation.append(llm_response.choices[0].message)   
        tool_calls = llm_response.choices[0].message.tool_calls or []
        for tool_call in tool_calls:
            print()
            print(f"[TOOL CALL] id:{tool_call.id} {tool_call.function.name} - {tool_call.function.arguments}")
            result = self.call_function_by_name(tool_call.function.name, tool_call.function.arguments)
            self.conversation.append({
                "role": "tool",
                "content": result,
                "tool_call_id": tool_call.id,
            })
            print(f"[TOOL RESPONSE] [id:{tool_call.id} name:{tool_call.function.name}] {result}")
            print()
        
        if self.candidate_response is None:
            self.candidate_response = llm_response.choices[0].message.content

        if self.candidate_response:
            self.conversation.append({
                "role": "user",
                "content": prompt,
            })
            self.conversation.append({
                "role": "assistant",
                "content": self.candidate_response,
            })

        if self.candidate_response is None:
            pass

            

    def respond(self, user_input: str) -> str:
        """Ask a question using LLM and update state."""

        self.candidate_response = None

        tool_choice = "auto"
        
        while self.candidate_response is None:
            llm_response = self.call_openai_llm(
                prompt=f"{user_input}",
                tools=FUNCTION_SCHEMAS,
                tool_choice=tool_choice,
            )
            self.handle_response(user_input, llm_response)
            if not self.candidate_response:
                print("No response, retrying...")
                tool_choice = {"type": "function", "function": {"name": "response_text"}}
            else:
                tool_choice = "auto"

        return self.candidate_response

        

        