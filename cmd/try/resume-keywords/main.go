package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"talent-rodeo/experimental/rodeo"

	"github.com/chriscow/minds"
	"github.com/chriscow/minds/providers/openai"
	"github.com/joho/godotenv"
)

const dsn = "db.sqlite"

func main() {
	if err := godotenv.Overload(); err != nil {
		log.Fatalf("failed to load .env file: %v", err)
	}

	// db, _ := database.New(dsn, false)
	// logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{Level: slog.LevelDebug}))

	// info, err := getCandidateInfo()
	// if err != nil {
	// 	log.Fatalf("failed to get candidate info: %v", err)
	// }

	// b, err := json.MarshalIndent(info, "", "  ")
	// if err != nil {
	// 	log.Fatalf("failed to marshal candidate info: %v", err)
	// }
	// log.Println(string(b))

	keywords, err := rodeo.GetResumeKeywords(fullResume)
	if err != nil {
		log.Fatalf("failed to get LinkedIn search terms: %v", err)
	}

	for _, kw := range keywords {
		fmt.Println(kw)
	}
}

func getCandidateInfo() (*rodeo.CandidateInfo, error) {
	schema, err := minds.NewResponseSchema("candidate_info", "Extracted candidate info from resume", rodeo.CandidateInfo{})
	if err != nil {
		return nil, fmt.Errorf("failed to generate schema: %v", err)
	}

	llm, err := openai.NewProvider(openai.WithResponseSchema(*schema))
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %v", err)
	}

	req := minds.NewRequest(
		minds.Messages{{Role: minds.RoleUser, Content: "I have a resume that I would like to extract candidate information from. Can you help me with that?\n\n" + fullResume}},
		minds.WithResponseSchema(*schema),
	)

	ctx := context.Background()
	resp, err := llm.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %v", err)
	}

	var info rodeo.CandidateInfo
	if err := json.Unmarshal([]byte(resp.String()), &info); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %v", err)
	}

	return &info, nil
}

const summaryResume = `
---
name: Chris Cowherd
phone: 425-829-5443
email: madcow@gmail.com
github: https://github.com/chriscow
linkedin: linkedin.com/in/chriscowherd
---

# Chris Cowherd

## Software Engineering Leader

**30 Years of Software Engineering Leadership**

- Extensive experience in a technical leadership role
- Engineering manager and hands-on contributor
- Thought leader, inventor, and implementer of protocol standards and systems
- Strategic, analytical, and highly personable, with a collaborative, inclusive leadership style
- Adept at seeing the big picture, building strong teams, and driving positive outcomes

**Core Values:** Transparent • Trustworthy • Inclusive • Accessible • Passionate

### Core Competencies

- Software Development
- Risk Management
- Privacy Compliance
- IT Platform Development
- Cloud Native Expertise
- Team Building / Leadership
- Information Security
- AI & Data Analytics
- Technology Diligence & Audits
- Financial Integration
- Acquisition & Transition
- Strategic Planning
- Tooling & Automation
- Vendor Management

## Professional Experience

### Backend Software Engineer, Generative AI  
**BlestX Ventures, LLC** - Redmond, WA | 2023 - Present  
Private research integrating generative AI for manufacturing processes  
**Stack:** OpenAI, Python, Go, Flask, Google Cloud, Docker  

- Integrated OpenAI technology for advanced workflows
- Hand-crafted PDF parsing and ingestion
- Developed retrieval augmented generation (RAG) using Weaviate & Pinecone
- Conducted prompt engineering and reranked results with cross-encoders
- Expanded search surface area with synthesized data
- Coordinated distributed LLM agents and optimized function calling

---

### Chief Technology Officer  
**i2 Holdings dba Ambassador Software** - Seattle, WA | 2020 - 2022  
Private equity funded SaaS company executing roll-up strategy in marketing technology AI & Analytics  
**Stack:** Python, Go, PostgreSQL, Django, Google Cloud, AWS, Docker  

- Protected and advanced all company technology assets
- Led a team of 18 engineers across geographies
- Owned product roadmaps, lifecycle design, and execution
- Delivered reliable project execution through meticulous planning
- Mentored team for professional growth and career development

---

### Principal Software Engineer  
**Stahurricane LLC** - Bellevue, WA | 2018 - 2021  
**Stack:** Go, Python, C#, CUDA, HLSL Compute Shaders, Unity, Google Cloud, AWS, Docker  

- Developed systems for VR and complex mathematical research
- Built cloud-native systems for ML inference at scale
- Explored higher-dimension structures with GPUs and Unity applications

---

### Chief Technology Officer  
**Donuts Inc.** - Bellevue, WA | 2012 - 2018  
World’s largest top-level domain registry operator  
**Stack:** Python, Go, C#, PostgreSQL, Django, Google Cloud, AWS, Docker  

- Managed full lifecycle for Internet-critical systems
- Successfully launched and operated over 200 TLDs
- Scaled growth and maintained ICANN-mandated uptime
- Grew the company 600x; #1 on Deloitte Fast 500

---

### Senior Engineering Manager, R&D  
**Demand Media (RightSide / Leaf Group)** - Kirkland, WA | 2008 - 2012  
**Stack:** C#, SQL Server, Redis  

- Managed a team of 12 cross-discipline engineers
- Designed the world's largest domain name registry

---

### Software Engineering Manager  
**eNom, Inc** - Redmond, WA | 2000 - 2008  
**Stack:** C++, C#, SQL Server  

- Guided a team of 7 engineers as a manager and contributor
- Designed scalable, fault-tolerant systems for DNS and other protocols

---

### Software Design Engineer  
**Microsoft** - Redmond, WA | 1997 - 2000  

- Contributed to early stages of Internet Explorer, Windows, and Office

---

### Programmer / Analyst  
**Advanced Semiconductor** - Phoenix, AZ | 1994 - 1996  

- Developed tools for semiconductor engineering processes

## Patents

- US11552923B2: Whitelist domain name registry
- US20160173440A1: Communication using DNS repurposing
- US20150100507A1: Domain Protected Marks List Service
- US20080273077: Virtual Video Message Answering Machine Platform
- US20080281676: Methods and Systems to Facilitate Keyword Bid Arbitrage

## Education

**Glendale College** - Mathematics / Computer Science | 1996
`

const fullResume = `
# CHRIS COWHERD
425-829-5443  
https://github.com/chriscow  
madcow@gmail.com  
linkedin.com/in/chriscowherd

## Software Engineering Leader
------------

**30 Years of Software Engineering Leadership**

- Extensive experience in a technical leadership role
- Engineering manager and hands-on contributor
- Thought leader, inventor and implementer of protocol standards and systems
- Strategic, analytical and highly personable, with a collaborative, inclusive leadership style
- Adept at seeing the big picture, building strong teams and driving positive outcomes

**Transparent • Trustworthy • Inclusive • Accessible • Passionate**

### CORE COMPETENCIES

Software Development | Team Building / Leadership | Acquisition & Transition
Risk Management | Information Security | Strategic Planning
Privacy Compliance | AI & Data Analytics | Tooling & Automation
IT Platform Development | Technology Diligence & Audits | Vendor Management
Cloud Native Expertise | Financial Integration

### PROFESSIONAL SUMMARY

#### BACKEND SOFTWARE ENGINEER, GENERATIVE AI
**BlestX Ventures, LLC - Redmond, WA (2023 - Present)**  
*Private research integrating generative AI for manufacturing processes*  
Stack: OpenAI, Python, Go, Flask, Google Cloud, Docker

- Integrate OpenAI technology
- Hand-crafted PDF parsing and ingestion
- Retrieval augmented generation (RAG) w/ Weaviate & Pinecone
- Prompt engineering
- Reranking results with cross-encoders
- Expand search surface area with synthesized data
- Customized workflow with extensive LLM function calling
- Distributed LLM agent coordination
- Fine tuning to optimize output of code generation

#### CHIEF TECHNOLOGY OFFICER
**i2 Holdings dba Ambassador Software - Seattle, WA (2020 - 2022)**  
*Private equity funded SaaS company executing roll-up strategy in marketing technology AI & Analytics*  
Stack: Python, Go, PostgreSQL, Django, Google Cloud, AWS, Docker, GDPR, CCPA & SOC2 Compliance

- Ultimate responsibility to protect, secure and advance all company technology assets
- Leader and champion for 18 geographically distributed engineers; onshore, nearshore and offshore
- Customer driven product owner, product roadmap development, strategic planning
- Full lifecycle owner and leader for design, development, testing, deployment and maintenance
- Coach and mentor for team career development and personal growth
- Reliable project execution and delivery time after time through careful planning, tracking and expectation management

#### PRINCIPAL SOFTWARE ENGINEER
**Stahurricane LLC - Bellevue, WA (2018 – 2021)**  
*Venture vehicle for ex-CEO of Donuts & eNom*  
Stack: Go, Python, C#, CUDA, HLSL Compute Shaders, Unity, Google Cloud, AWS, Docker

- R&D software design & development of VR and mathematical research
- Designed and developed distributed system to explore and visualize difficult to solve mathematical areas using cloud GPUs (see: https://zeta-machine.chriscowherd.com/)
- Designed and developed distributed cloud-native system to perform ML inference at scale (see: https://github.com/chriscow/cloud-scanner-go)
- Designed and developed distributed brute force system to discover fundamental constants (see: https://github.com/chriscow/ramanujan-machine)
- Designed and developed application in Unity to use GPUs to visualize and explore higher dimension structures (12 dimension) and project those down to 4-dimensions then a 2-dimensional plane

#### CHIEF TECHNOLOGY OFFICER
**Donuts Inc. - Bellevue, WA (2012 - 2018)**  
*World's largest top-level domain registry operator*  
Stack: Python, Go, C#, PostgreSQL, Django, Google Cloud, AWS, Docker, GDPR Compliance

- Provide critical functionality for proper functioning of the Internet naming infrastructure
- Manage full lifecycle for Internet-critical software systems, quality assurance, network & ops
- Scale growth while maintaining ICANN mandated uptime for global Internet infrastructure
- Successfully launched and operated over 200 TLDs without incident
- Grew 600x from '13 – '16; named #1 on Deloitte Fast 500 as fastest growing company in No. America

**Key responsibilities:**
- Corporate Technology / Product Development / Incident Management / Risk Management
- Operations, security, budgeting, technology audits, business continuity and M&A diligence
- Team building and leadership in and across organizations

**Management Portfolio:** (15 direct reports, organization of 45)
- Platform Development
- Quality assurance
- Corporate IT & facilities
- Production engineering
- Tooling and automation
- Data warehousing, analytics & AI
- Business intelligence
- Operations & technical support
- Financial backend integration
- Corporate & Physical Security

#### SENIOR ENGINEERING MANAGER, R&D
**Demand Media (RightSide / Leaf Group) - Kirkland, WA (2008 – 2012)**  
*Content & media generation, monetization*  
Stack: C#, SQL Server, Redis

- Leader, manager and mentor for 12 cross-discipline engineers
- Develop many successful POCs by fostering creativity and input from all team members
- Full SDLC using agile methodologies and continuous integration
- Design and initial implementation of the world's largest domain name registry

#### SOFTWARE ENGINEERING MANAGER
**eNom, Inc - Redmond, WA (2000 – 2008)**  
*Wholesale white-label platform for domains, email, and SSL*  
Stack: C++, C#, SQL Server

Software Engineering Mgr 2004 - 2008  
Principal Software Engineer 2000 – 2004

- Managed and coached team of 7 engineers while individual contributor
- Devised scalable server architectures for long running services such as DNS
- Networking and Internet protocol development for DNS, SMTP, POP, IMAP etc.
- Designed scalable, fault tolerant systems
- Guided and mentored team through uncertainty

#### SOFTWARE DESIGN ENGINEER
**Microsoft - Redmond, WA (1997 - 2000)**  
*World class development experience, including key contributions to early stages of Internet Explorer, Windows and Office*

#### PROGRAMMER / ANALYST
**Advanced Semiconductor - Phoenix, AZ (1994 - 1996)**  
*Designed and developed tools for managing engineering changes to semiconductor processing equipment*

### PATENTS

- U.S. Patent # US11552923B2  
  Whitelist domain name registry

- U.S. Patent # US20160173440A1  
  Communication using dns repurposing

- U.S. Patent # US20150100507A1  
  Domain Protected Marks List Service

- U.S. Patent # US20080273077  
  Virtual Video Message Answering Machine Platform

- U.S. Patent # US20080281676  
  Methods and Systems to Facilitate Keyword Bid Arbitrage with Multiple Advertisement Placement Providers

### EDUCATION

**Glendale College**  
Mathematics / Computer Science (1996)`
