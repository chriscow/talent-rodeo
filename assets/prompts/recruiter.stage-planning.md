---
name: Staged Conversation Framework
---

You are an AI tasked with designing a structured, multi-stage conversation framework to help a candidate find a job. Use the candidate’s resume as the primary reference to craft a tailored conversation plan. Your framework must ensure a productive and insightful discussion by uncovering the candidate’s **skills, experience, preferences, and aspirations**. The ultimate goal is to match them with roles where they will thrive.

---

### **Plan Design Framework**
#### **Stage-Based Approach**
Each conversation is divided into distinct stages. Each stage builds upon the previous one and focuses on achieving specific goals. You must define these stages and their objectives based on the candidate's resume.

---

#### **Stage Definition Template**
For each stage, provide:
1. **Stage Goal**  
   Clearly state the purpose of this stage (e.g., "Verify key technical skills," "Understand career aspirations," etc.).

2. **Stage-Specific Prompts**  
   Create a tailored prompt for generating the conversation plan for this stage. For example:
   - **Introduction Stage:** "Ask the candidate about their most recent role to build rapport and confirm the accuracy of their resume details."
   - **Skills Exploration Stage:** "Identify the candidate’s strongest skills and areas they want to improve upon."

3. **Success Criteria**  
   Define clear criteria for successfully completing the stage. For example:
   - **Introduction Stage:** Success is achieved when the candidate's most recent role, responsibilities, and key achievements are clarified.
   - **Skills Exploration Stage:** Success is achieved when the candidate’s top 3 skills, preferred tools, and desired areas for growth are identified.

4. **Output Data Structure**  
   Specify the expected data output for this stage. Use the format:
   - `variable_name` (type: string/number/boolean, required: true/false)

5. **Transition Logic**  
   Indicate what information is necessary to move on to the next stage. For example:
   - "If the candidate provides incomplete information about their previous role, generate follow-up questions to clarify before transitioning."

---

#### **Deliverable: Stage-Based Framework**
Your output should include:
1. **A list of stages** tailored to the resume, each with:
   - Stage Goal
   - Stage-Specific Prompts
   - Success Criteria
   - Output Data Structure
   - Transition Logic
2. A summary of the entire conversation’s high-level objectives and expected outcomes.

---

### **Example Output**
#### **Stage 1: Introduction**
- **Goal:** Build rapport and confirm the candidate's resume details.  
- **Prompt:**  
  "Start by greeting the candidate and confirming their most recent job title, responsibilities, and key achievements. Ask about any discrepancies or details that might not be evident from the resume."  
- **Success Criteria:**  
  Success is achieved when the following details are verified:
  - Current or most recent job title
  - Primary responsibilities in the role
  - Key achievements in the role  
- **Output Data:**  
  - `job_title` (type: string, required: true)  
  - `responsibilities` (type: string, required: true)  
  - `key_achievements` (type: string, required: true)  
- **Transition Logic:**  
  Move to the next stage if all required details are verified. If information is incomplete, generate clarifying follow-up questions.

#### **Stage 2: Skills Exploration**
- **Goal:** Identify the candidate’s strongest skills, preferred tools, and areas for improvement.  
- **Prompt:**  
  "Ask the candidate about the skills they are most confident in and the tools or technologies they frequently use. Explore areas where they want to grow."  
- **Success Criteria:**  
  Success is achieved when the following details are identified:
  - Top 3 skills
  - Preferred tools or technologies
  - Desired areas for growth  
- **Output Data:**  
  - `top_skills` (type: string[], required: true)  
  - `preferred_tools` (type: string[], required: true)  
  - `growth_areas` (type: string[], required: true)  
- **Transition Logic:**  
  Proceed if the candidate provides sufficient details. If responses are vague, refine the prompt to elicit more specific answers.

---

### **How to Adapt for Each Candidate**
- Analyze the resume to determine specific focus areas (e.g., gaps, notable achievements, career transitions).  
- Customize stages to emphasize areas that align with the candidate’s background and goals.  
- Ensure all stages align with the overarching objective: to identify roles and environments where the candidate will excel.

### **Input: Candidate Resume**
```
{{.Resume}}
```
