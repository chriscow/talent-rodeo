import openai
import json

import prompt


class Candidate:
    client: openai.OpenAI
    system_template: str

    def __init__(self, openai_api_key: str, name="Alex Chen", profile=None):
        openai.api_key = openai_api_key
        self.client = openai.OpenAI()

        self.name = name
        self.profile = profile or {
            "core_skills": ["Python", "FastAPI", "Microservices"],
            "experience_level": "Senior Backend Engineer",
            "tech_stack_preferences": ["AWS", "Kafka", "PostgreSQL"],
            "ideal_role": "A backend engineering position with a focus on scalable systems.",
            "current_situation": "Working at TechCorp Inc.",
            "motivation_for_change": "Seeking new challenges and opportunities for growth.",
            "start_timeline": "Within the next two months.",
        }
        self.state = {"answered_questions": []}

        self.system_template, header = prompt.from_template(
            "prompts/candidate_python_dev.txt",
            context={"state": self.state},
        )

    def call_openai_llm(self, recruiter_question: str) -> str:
        """Generate a response to the Recruiter's question using GPT-4."""

        messages = [
            {"role": "system", "content": self.system_template.render()},
            {"role": "user", "content": recruiter_question},
        ]

        completion = self.client.chat.completions.create(
            model="gpt-4o-mini",
            messages=messages,
        )
        return completion.choices[0].message.content

    def answer_question(self, recruiter_question: str) -> str:
        """Generate and return the Candidate's response."""
        candidate_response = self.call_openai_llm(recruiter_question)
        return candidate_response
