from openai import OpenAI
from dotenv import load_dotenv
import os, json

from recruiter import Recruiter
from enhanced_recruiter import EnhancedRecruiter, CallStatus
from candidate import Candidate


load_dotenv()
client = OpenAI()

if __name__ == "__main__":
    load_dotenv()
    api_key = os.getenv("OPENAI_API_KEY")
    if not api_key:
        raise ValueError("Please set the OPENAI_API_KEY environment variable.")

    recruiter = EnhancedRecruiter(
        openai_api_key=api_key,
        system_template_path="prompts/recruiter.system.002.txt",
    )
    candidate = Candidate(openai_api_key=api_key)

    response = "Hello?"

    for i in range(15):
        response = recruiter.respond(response)
        print("[RECRUITER] ", response)
        print()

        if recruiter.state.call_context.status == CallStatus.COMPLETED:
            break

        response = candidate.answer_question(response)
        print("[CANDIDATE] ", response)
        print()

    # save the recruiter state to a file
    with open("recruiter_state.json", "w") as f:
        f.write(recruiter.state.model_dump_json())

    with open("recruiter_conversation.json", "w") as f:
        f.write(json.dumps(recruiter.conversation, indent=4))
