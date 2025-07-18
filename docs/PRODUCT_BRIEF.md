# Talent Rodeo Product Brief

## Executive Summary
Talent Rodeo is an AI-powered job search platform that revolutionizes how professionals find their next opportunity through three core value propositions: 1) We understand you better than anyone else through natural conversation that captures your career story beyond just your resume, 2) We filter out the noise by analyzing thousands of jobs daily to surface only opportunities worth your time and attention, and 3) We make sure you never miss your perfect opportunity through daily deep analysis and instant notifications. The platform leverages Large Language Models (LLMs) to create a personalized, high-touch experience that saves job seekers countless hours while ensuring they never miss their ideal role.

## Product Vision
To create an intelligent job search platform that understands professionals through natural conversation rather than forms and keywords, filters signal from noise by analyzing thousands of daily job postings, and ensures candidates never miss their perfect opportunity through timely analysis and notifications.

## Target Market
Primary users fall into two categories:

Job Seekers:
- Professionals actively seeking new opportunities
- Passive candidates open to compelling opportunities
- Career changers looking for guidance and connections

Recruiters:
- Independent recruiters and agencies
- Corporate hiring managers
- HR professionals

## Core Features

### Candidate Engagement
The system saves candidates many hours of reading job descriptions by pre-screening job listings. The candidate logs into the platform via LinkedIn social login (OAuth 2.0). The candidate leaves their phone number and uploads their resume as well as indicates a good time for the system to call. The system AI calls the candidate on the phone to build a comprehensive candidate profiles The system:

- Conducts initial screening calls to understand candidate motivations and preferences
- Schedules regular check-in calls to maintain engagement and track progress
- Uses natural conversation to uncover traits and experiences not apparent on resumes
- Maintains detailed profiles of candidates' skills, preferences, and career goals

### Automated Communication
- AI Voice synthesis for natural phone conversations
- Automated email follow-ups for missed calls
- Regular check-ins and status updates
- Custom notifications for matching opportunities

### Profile Enhancement
- Resume analysis and optimization
- Custom resume tailoring for specific opportunities
- Cover letter generation
- Skills and experience validation through conversation

### Matching System
- Intelligent job-candidate matching using LLM analysis
- Custom candidate presentations for recruiters
- Automated introduction facilitation
- Match scoring and ranking

### Premium Features
- Priority access to new job listings
- Optional Email and SMS notifications as soon as a new job is listed that matches
- Unlimited on-demand agent calls
- Advanced resume optimization and tailored cover letter generation in their own style
- Customized interview preparation
- Enhanced profile visibility to recruiters

## Technical Architecture

### Backend Implementation
- Go-based server implementation
- HTML templating using html/template package
- PicoCSS for styling
- Flow router with nested middleware groups
- OpenAI Realtime API for LLM and agent voice
- Twilio for incoming and outgoing phone calls

### Security Layers
1. Global Middleware
   - Panic recovery
   - Security headers
   - Request logging

2. Base Group
   - CSRF protection
   - Authentication validation

3. Access-Level Groups
   - Anonymous access (auth flows)
   - Authenticated access (user operations)
   - Superuser access (admin features)

### Database Schema
- PostgreSQL for primary data storage
- Structured for efficient matching and search
- Secure handling of sensitive information
- Audit logging for all interactions

### Integration Points
- LinkedIn API for initial contact
- Voice synthesis service for calls
- Email service for notifications
- Payment processing for premium features
- Calendar integration for scheduling

## Revenue Model
- 7-day free trial with full access to all features
- Premium subscription for continued access after trial
- Per-introduction fee for recruiters
- Success-based fees for placements
- Enterprise pricing for high-volume recruiters

## Success Metrics
- Candidate engagement rates
- Successful placement ratio
- User retention metrics
- Premium conversion rate
- Recruiter satisfaction scores
- Platform response times
- Match accuracy ratings