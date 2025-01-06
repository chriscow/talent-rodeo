import openai
import json, uuid
from enum import Enum
from datetime import datetime, timedelta
from pydantic import BaseModel, Field
from typing import List, Dict, Optional
from jinja2 import Template

import prompt

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
]


# Enums
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


class ConversationGoal(str, Enum):
    ESTABLISH_RAPPORT = "establish_rapport"
    GATHER_BACKGROUND = "gather_background"
    UNDERSTAND_MOTIVATIONS = "understand_motivations"
    IDENTIFY_PREFERENCES = "identify_preferences"
    DISCUSS_SKILLS = "discuss_skills"
    VALIDATE_EXPERIENCE = "validate_experience"
    EXPLORE_INTERESTS = "explore_interests"
    PLAN_NEXT_STEPS = "plan_next_steps"


class CallStatus(str, Enum):
    NOT_STARTED = "not_started"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    MISSED = "missed"
    SCHEDULED = "scheduled"


# Data Models
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


class GoalProgress(BaseModel):
    goal: ConversationGoal
    status: bool = False
    related_info: List[str] = Field(default_factory=list)
    priority: int = 1


class SkillInfo(BaseModel):
    name: str
    proficiency: str
    last_used: Optional[str]
    years_experience: Optional[float]
    projects: List[str] = Field(default_factory=list)
    interests: List[str] = Field(default_factory=list)


class CandidateProfile(BaseModel):
    core_skills: List[SkillInfo] = Field(default_factory=list)
    desired_skills: List[str] = Field(default_factory=list)
    avoid_skills: List[str] = Field(default_factory=list)
    career_aspirations: List[str] = Field(default_factory=list)
    work_preferences: Dict[str, str] = Field(default_factory=dict)
    achievement_highlights: List[str] = Field(default_factory=list)


class CallContext(BaseModel):
    today: str = datetime.now().isoformat()
    call_id: str
    start_time: Optional[datetime]
    duration: Optional[timedelta]
    status: CallStatus = CallStatus.NOT_STARTED
    goals_progress: List[GoalProgress] = Field(default_factory=list)
    completion_criteria: Dict[str, bool] = Field(default_factory=dict)


class ConversationState(BaseModel):
    stage: ConversationStage
    candidate_info: CandidateInfo
    conversation_context: ConversationContext
    call_context: CallContext
    candidate_profile: CandidateProfile = Field(default_factory=CandidateProfile)

    def initialize_call(self) -> None:
        self.call_context.goals_progress = [
            GoalProgress(goal=ConversationGoal.ESTABLISH_RAPPORT, priority=1),
            GoalProgress(goal=ConversationGoal.GATHER_BACKGROUND, priority=2),
            GoalProgress(goal=ConversationGoal.UNDERSTAND_MOTIVATIONS, priority=3),
            GoalProgress(goal=ConversationGoal.DISCUSS_SKILLS, priority=4),
            GoalProgress(goal=ConversationGoal.PLAN_NEXT_STEPS, priority=5),
        ]
        self.call_context.completion_criteria = {
            "basic_info_collected": False,
            "career_goals_discussed": False,
            "skills_assessed": False,
            "next_steps_planned": False,
        }

    def update_goal_progress(self, goal: ConversationGoal, info: List[str]) -> None:
        for g in self.call_context.goals_progress:
            if g.goal == goal:
                g.related_info.extend(info)
                if len(g.related_info) >= 2:
                    g.status = True
                break

    def should_end_call(self) -> bool:
        return all(goal.status for goal in self.call_context.goals_progress)

    def get_next_goal(self) -> Optional[ConversationGoal]:
        incomplete_goals = [g for g in self.call_context.goals_progress if not g.status]
        if not incomplete_goals:
            return None
        return min(incomplete_goals, key=lambda x: x.priority).goal


# Recruiter Class
class Recruiter:
    def __init__(self, openai_api_key: str, system_template_path: str):
        openai.api_key = openai_api_key
        self.client = openai.OpenAI()
        self.function_map = {}
        self.conversation = []
        self.state = self.initialize_state()
        self.system_template, _ = prompt.from_template(
            system_template_path, context={"state": self.state}
        )

    def initialize_state(self) -> ConversationState:
        return ConversationState(
            stage=ConversationStage.INITIAL_CONTACT,
            candidate_info=CandidateInfo(),
            conversation_context=ConversationContext(),
            call_context=CallContext(call_id=str(uuid.uuid4())),
        )

    def call_function_by_name(self, func_name: str, json_args: str):
        args = json.loads(json_args)
        func = getattr(self, func_name, None)
        if not func:
            raise ValueError(f"Function '{func_name}' not found.")
        return func(**args)

    def respond(self, user_input: str) -> str:
        messages = [
            {
                "role": "system",
                "content": self.system_template.render(
                    {"today": datetime.now().isoformat(), "state": self.state}
                ),
            },
            {"role": "user", "content": user_input},
        ]
        completion = self.client.chat.completions.create(
            model="gpt-4o-mini", messages=messages
        )
        return completion.choices[0].message.content
