
1a. Search Algorithm

## Step 1: Parse user preferences into query groups
First, categorize the skills based on their ratings:
- Must Have (5): Skills that are required and should be primary
- Prefer (4): Skills that are desired but not necessarily primary
- No Preference (3): Optional skills
- Avoid (2): Skills to potentially exclude, but with flexibility
- Must Avoid (1): Skills to definitely exclude

## Step 2: Construct primary search queries

**Initial Queries:**
1. **Core Query**: Search for job descriptions containing ALL "Must Have" skills.

2. **Extended Query**: Include jobs with "Must Have" AND "Prefer" skills.

## Step 3: Process exclusions
Filter out jobs that contain "Must Avoid" skills:

## Step 4: Result ranking and scoring
For each job in the results:
1. Calculate a base score:
   - +10 points for each "Must Have" skill match
   - +5 points for each "Prefer" skill match
   - +1 point for each "No Preference" skill match
   - -3 points for each "Avoid" skill that appears
   - Immediate exclusion for "Must Avoid" matches

2. Use the LLM to analyze job descriptions to determine (/internal/handlers/assessment.go, /internal/handlers/requirements.go):
   - Is the "Must Have" skill truly primary to the role? (weight heavily)
   - What percentage of the job involves each skill? (adjust score accordingly)

## Step 5: Execute search strategy in sequence

1. Run the Core Query (Must Have skills only).
2. Run the Extended Query (Must Have + Prefer).
3. For each result set:
   - Remove jobs from consideration containing Must Avoid skills.
   - Send job descriptions to the LLM for deeper analysis.
   - Calculate final score based on keyword matches and LLM analysis.

## Step 6: Merge and present results

1. Combine the results from different queries, removing duplicates.
2. Sort by final score in descending order.
3. Group results into categories:
   - "Perfect Matches" (high scores with primary Must Have skills)
   - "Strong Matches" (good scores with Must Have skills present)
   - "Potential Matches" (lower scores but still meeting basic criteria)

## Step 7: LLM-enhanced job description analysis

For each job description that passes the initial filtering:
1. Send the job description to the LLM with a prompt like:
   ```
   Analyze this job description for the following skills:
   - Must Have: Go
   - Prefer: Python
   - Must Avoid: React, Vue, TypeScript
   
   Determine:
   1. Is Go the primary language/skill required?
   2. What percentage of the job involves each skill?
   3. Are any Must Avoid skills central to the role?
   ```

2. Parse the LLM response to:
   - Adjust scores based on how central the Must Have skills are
   - Further exclude jobs where Must Avoid skills are central
   - Generate a brief summary of skill relevance for the user

