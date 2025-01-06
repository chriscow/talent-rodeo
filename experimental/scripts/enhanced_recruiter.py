import openai

# from openai.types.completion import ChatCompletion
import json, yaml
from enum import Enum
from dataclasses import dataclass, field, asdict
from jinja2 import Template
from datetime import datetime, timedelta
from enum import Enum
from pydantic import BaseModel, Field
from typing import List, Dict, Optional

import prompt

from recruiter import Recruiter, ConversationState, FUNCTION_SCHEMAS


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


class EnhancedConversationState(ConversationState):
    call_context: CallContext
    candidate_profile: CandidateProfile = Field(default_factory=CandidateProfile)

    def initialize_call(self) -> None:
        """Set up initial call goals and completion criteria"""
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
        """Update progress for a specific conversation goal"""
        for g in self.call_context.goals_progress:
            if g.goal == goal:
                g.related_info.extend(info)
                if len(g.related_info) >= 2:  # Simple completion criteria
                    g.status = True
                break

    def should_end_call(self) -> bool:
        """Determine if call objectives have been met"""
        return all(goal.status for goal in self.call_context.goals_progress)

    def get_next_goal(self) -> Optional[ConversationGoal]:
        """Get the highest priority incomplete goal"""
        incomplete_goals = [g for g in self.call_context.goals_progress if not g.status]
        if not incomplete_goals:
            return None
        return min(incomplete_goals, key=lambda x: x.priority).goal


# Add new goal tracking function schemas
ADDITIONAL_FUNCTION_SCHEMAS = [
    {
        "type": "function",
        "function": {
            "name": "update_goal_progress",
            "description": "Update progress for a conversation goal",
            "parameters": {
                "type": "object",
                "properties": {
                    "goal": {
                        "type": "string",
                        "enum": [e.value for e in ConversationGoal],
                        "description": "The conversation goal to update",
                    },
                    "info": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "Information gathered related to this goal",
                    },
                },
                "required": ["goal", "info"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_candidate_profile",
            "description": "Update the candidate's profile with new information",
            "parameters": {
                "type": "object",
                "properties": {
                    "core_skills": {
                        "type": "array",
                        "items": {
                            "type": "object",
                            "properties": {
                                "name": {"type": "string"},
                                "proficiency": {"type": "string"},
                                "years_experience": {"type": "number"},
                            },
                        },
                    },
                    "desired_skills": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                    "avoid_skills": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                    "career_aspirations": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                },
            },
        },
    },
]


# Update Recruiter class
class EnhancedRecruiter(Recruiter):
    def __init__(self, openai_api_key: str, system_template_path: str):
        super().__init__(openai_api_key, system_template_path=system_template_path)
        self.state = self.initialize_enhanced_state()

        # Add new functions to function map
        self.function_map.update(
            {
                "update_goal_progress": self.update_goal_progress,
                "update_candidate_profile": self.update_candidate_profile,
            }
        )

        # Update function schemas
        global FUNCTION_SCHEMAS
        FUNCTION_SCHEMAS.extend(ADDITIONAL_FUNCTION_SCHEMAS)

    def initialize_enhanced_state(self) -> EnhancedConversationState:
        base_state = self.initialize_state()

        # Create call context
        call_context = CallContext(
            today=datetime.now().isoformat(),
            call_id="initial_call",
            start_time=None,
            duration=None,
            status=CallStatus.NOT_STARTED,
        )

        # Create enhanced state
        enhanced_state = EnhancedConversationState(
            stage=base_state.stage,
            candidate_info=base_state.candidate_info,
            conversation_context=base_state.conversation_context,
            call_context=call_context,
            candidate_profile=CandidateProfile(),
        )

        # Initialize call goals
        enhanced_state.initialize_call()

        return enhanced_state

    def update_goal_progress(
        self, state: EnhancedConversationState, goal: str, info: List[str]
    ) -> str:
        """Update progress for a specific conversation goal"""
        goal_enum = ConversationGoal(goal)
        state.update_goal_progress(goal_enum, info)

        # Check if call should end
        if state.should_end_call():
            self.end_call(state)

        return state.call_context.model_dump_json()

    def update_candidate_profile(
        self, state: EnhancedConversationState, **profile_updates
    ) -> str:
        """Update the candidate's profile with new information"""
        if "core_skills" in profile_updates:
            for skill_data in profile_updates["core_skills"]:
                skill = SkillInfo(**skill_data)
                if skill not in state.candidate_profile.core_skills:
                    state.candidate_profile.core_skills.append(skill)

        if "desired_skills" in profile_updates:
            state.candidate_profile.desired_skills.extend(
                skill
                for skill in profile_updates["desired_skills"]
                if skill not in state.candidate_profile.desired_skills
            )

        if "avoid_skills" in profile_updates:
            state.candidate_profile.avoid_skills.extend(
                skill
                for skill in profile_updates["avoid_skills"]
                if skill not in state.candidate_profile.avoid_skills
            )

        if "career_aspirations" in profile_updates:
            state.candidate_profile.career_aspirations.extend(
                aspiration
                for aspiration in profile_updates["career_aspirations"]
                if aspiration not in state.candidate_profile.career_aspirations
            )

        return state.candidate_profile.model_dump_json()

    def respond(self, user_input: str) -> str:
        """Enhanced response method with goal tracking"""
        self.state.call_context.today = datetime.now().isoformat()
        if self.state.call_context.start_time:
            self.state.call_context.duration = (
                datetime.now() - self.state.call_context.start_time
            )
        else:
            self.state.call_context.status = CallStatus.IN_PROGRESS
            self.state.call_context.start_time = datetime.now()

        response = super().respond(user_input)

        # Get next goal if available
        next_goal = self.state.get_next_goal()
        if next_goal:
            self.state.conversation_context.next_response_goals.append(
                f"Progress on goal: {next_goal.value}"
            )

        return response

    def end_call(self, state: EnhancedConversationState) -> str:
        """Enhanced call ending with summary"""
        state.call_context.status = CallStatus.COMPLETED

        # Create call summary
        summary = {
            "completed_goals": [
                goal.goal.value
                for goal in state.call_context.goals_progress
                if goal.status
            ],
            "gathered_info": state.candidate_info.known_facts,
            "follow_ups": state.conversation_context.follow_up_items,
        }

        # Add summary to conversation context
        state.conversation_context.knowledge.append(
            f"Call completed with summary: {json.dumps(summary, indent=2)}"
        )

        return "OK"
