package rodeo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

func StartWorkflow(ctx context.Context, sessionID, resumeText string, status StatusSender) error {
	if status == nil {
		status = defaultStatus{}
	}

	//
	// The workflow generally consists of:
	// 1. Convert resume to YAML
	// 2. Generate a conversation plan for the recruiter
	// 3. Create a system prompt for the recruiter that includes the plan and resume
	// 4. Call the candidate
	//
	// llm, err := openai.NewProvider()
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to create Gemini provider")
	// 	return fmt.Errorf("analyzeResume: failed to create Gemini provider: %w", err)
	// }

	// resumeYAML, err := convertResume(ctx, llm, resumeText, sessionID, status)
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to convert resume")
	// 	return fmt.Errorf("startWorkflow: failed to convert resume: %w", err)
	// }

	// plan, err := generatePlan(ctx, llm, resumeText)
	// if err != nil {
	// 	status.SendStatus(sessionID, "error", "Failed to generate plan")
	// 	return fmt.Errorf("startWorkflow: failed to generate plan: %w", err)
	// }

	prompt, err := CreateTemplate("prompts/recruiter.system.003.txt")
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to create prompt template")
		return fmt.Errorf("startWorkflow: failed to create prompt template: %w", err)
	}

	systemPrompt := bytes.NewBuffer([]byte(plan))
	data := map[string]any{
		"Plan":   plan,
		"Resume": resumeYAML,
	}
	if err := prompt.Template.Execute(systemPrompt, data); err != nil {
		status.SendStatus(sessionID, "error", "Failed to execute prompt template")
		return fmt.Errorf("startWorkflow: failed to execute prompt template: %w", err)
	}

	info, err := callCandidate(ctx, sessionID, systemPrompt.String(), status)
	if err != nil {
		status.SendStatus(sessionID, "error", "Failed to call candidate")
		return fmt.Errorf("startWorkflow: failed to call candidate: %w", err)
	}

	status.SendStatus(sessionID, "success", "Workflow completed successfully!")

	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("startWorkflow: failed to marshal JSON: %w", err)
	}

	fmt.Println(string(b))

	return nil

	// prompt := "Hello?"

	// text, _ := resp.Text()
	// println("R > " + text)
	// tc := minds.NewThreadContext(ctx)
	// tc.AppendMessages(minds.Message{
	// 	Role:    minds.RoleSystem,
	// 	Content: systemPrompt.String(),
	// })

	// return autoChat(tc, 25)
}

const plan = `### **Conversation Plan for Chris Cowherd**
#### **1. Conversation Goals**
- **Confirm Technical Expertise:** Verify the strengths in programming languages, technologies, and tools as outlined in the resume.
- **Understand Work Preferences and Aspirations:** Explore the candidate's preferences for work environments, ideal roles, industries, and career goals.
- **Identify Unlisted Skills and Experiences:** 
	Encourage Chris to share any additional skills, projects, interests, or 
	aspirations that are not included in the resume.
	
#### **2. Develop Subgoals**
- **Technical Expertise:**
  - Ask clarifying questions about programming languages, particularly Python and Go.
  - Discuss familiarity and comfort with current technology stacks, especially 
  related to cloud technologies and Docker.
  - Explore any new tools or technologies they wish to learn or implement in the future.
- **Preferences:**
  - Assess preferences for team dynamics, such as ideal team size and structure.
  - Discuss preferred working styles (remote, hybrid, in-office) and flexibility in hours.
  - Identify desired industry sectors and types of companies Chris aims to work with.
  
- **Unlisted Skills:**
  - Prompt the candidate to share about any personal or side projects they've undertaken.
  - Inquire into any certifications, training, or learning experiences not highlighted in the resume.
  
#### **3. Generate Exploration Themes**
- **Technical Skills & Tools:** 
  - Delve into expertise in Python, Go, and cloud technologies.
  - Discuss recent projects and technologies (e.g., OpenAI, Docker) and their implications on Chris' career interests.
  
- **Soft Skills & Leadership:** 
  - Explore experiences demonstrating transparent leadership and strategic thinking.
  - Discuss examples of team-building and collaboration and how these skills influenced past success.
  
- **Career Accomplishments:** 
  - Discuss monumental achievements such as scaling projects and successful launches and their impact on Chris’ career path.
  
- **Work-Life Balance and Expectations:**
  - Discuss preferences for hybrid/remote work, salary expectations, and any deal-breakers in job offers.
  
#### **4. Outline Potential Outcomes**
- **Skills and Goals Summary:** A clear picture of Chris' technical skills and career aspirations.
- **Role and Industry Interest:** Identification of specific roles, industries, or companies Chris is most excited about.
- **Knowledge of Ideal Environment:** Insights into the type of work environment that aligns with Chris’ values and lifestyle.

---

### **Dynamic Open-Ended Questions**
- **Technical Skills:**
  - \"Can you share a recent project where you predominantly used Python or Go? What challenges did you face, and how did you overcome them?\"
  - \"What new technologies or tools are you currently learning or interested in exploring further?\"
- **Work Preferences:**
  - \"Describe your ideal team environment. What qualities do you value most in your teammates or leadership?\"
  - \"What type of company culture do you believe best suits your working style and professional goals?\"
  
- **Unlisted Skills:**
  - \"Are there any side projects or contributions to open-source that you're particularly proud of? How have these experiences influenced your career?\"
  - \"Have you pursued any additional certifications or trainings that might not be reflected in your resume?\"
  
- **Career Aspirations:**
  - \"Looking towards the future, what does your next career milestone look like? What kind of roles or responsibilities are you aiming for?\"
  - \"How important is work-life balance to you, and what expectations do you have regarding work arrangements in your next role?\"

By following this structure, the conversation will remain flexible and responsive 
to Chris Cowherd’s insights, optimizing the chance for a productive, supportive 
dialogue that leads to viable job opportunities.`

const resumeYAML = `name: Chris Cowherd
contact:
  phone: 425-829-5443
  email: madcow@gmail.com
  github: https://github.com/chriscow
  linkedin: linkedin.com/in/chriscowherd
summary:
  title: Software Engineering Leader
  experience_years: 30
  description: >
    Extensive experience in technical leadership, software development, cloud-native systems, AI & data analytics,
    and scaling teams and products. Highly strategic, analytical, and collaborative.
soft_skills:
  - name: Transparent Leadership
    ai_comment: Strongly demonstrated through inclusive team management; active in large-scale cross-functional teams.
  - name: Strategic Thinking
    ai_comment: Evident in career focus on scaling, compliance, and operational excellence.
  - name: Team Building
    ai_comment: Established and led teams of varying sizes across distributed regions.
programming_languages:
  - name: Python
    ai_comment: Expert level, recently used in 2023 for backend and generative AI development.
  - name: Go
    ai_comment: Advanced, used frequently in recent roles for backend and distributed systems.
  - name: C#
    ai_comment: Proficient, with significant use in earlier roles but less recent.
  - name: C++
    ai_comment: Competent, utilized in earlier career roles.
technology_stacks:
  - name: OpenAI Integration
    ai_comment: Currently employed in RAG pipelines, LLM optimization, and function orchestration.
  - name: Cloud Technologies (AWS, GCP)
    ai_comment: Extensive experience in deployment, scaling, and compliance.
  - name: Docker
    ai_comment: Used across roles for containerized deployments; expertise evident.
  - name: SQL Databases (PostgreSQL, SQL Server)
    ai_comment: Highly experienced in schema design, optimization, and compliance.
key_accomplishments:
  - description: >
      Launched and operated over 200 TLDs without incident, achieving 600x growth from 2013 to 2016.
    ai_comment: Indicates exceptional ability to manage large-scale projects with high operational demands.
  - description: Developed distributed systems for advanced mathematical and AI research.
    ai_comment: Demonstrates technical innovation and R&D expertise.
  - description: Holder of multiple patents in DNS technology and advertisement systems.
    ai_comment: Reflects thought leadership and innovation in the software domain.
education:
  - institution: Glendale College
    field_of_study: Mathematics / Computer Science
    graduation_year: 1996
    ai_comment: Solid technical foundation supporting a career of engineering excellence.
professional_experience:
  - role: Backend Software Engineer, Generative AI
    company: BlestX Ventures, LLC
    location: Redmond, WA
    years_active: 2023-Present
    responsibilities: >
      Leading research in generative AI for manufacturing processes, integrating OpenAI, and fine-tuning LLMs.
    ai_comment: Recently active; reflects cutting-edge engagement with AI technologies.
  - role: Chief Technology Officer
    company: i2 Holdings
    location: Seattle, WA
    years_active: 2020-2022
    responsibilities: >
      Oversaw SaaS roll-ups in marketing tech AI; managed compliance, roadmaps, and team growth.
    ai_comment: Strong leadership in SaaS and compliance initiatives.
  - role: Principal Software Engineer
    company: Stahurricane LLC
    location: Bellevue, WA
    years_active: 2018-2021
    responsibilities: >
      Designed systems for VR, mathematical visualization, and distributed ML inference.
    ai_comment: Demonstrates deep technical skills and innovation.
patents:
  - US11552923B2: Whitelist domain name registry
  - US20160173440A1: Communication using DNS repurposing
  - US20150100507A1: Domain Protected Marks List Service
  - US20080273077: Virtual Video Message Answering Machine Platform
  - US20080281676: Methods and Systems to Facilitate Keyword Bid Arbitrage`
