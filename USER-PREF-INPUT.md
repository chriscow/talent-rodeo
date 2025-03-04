# Implementation Plan for Job Preferences and Search Criteria

## Current Status

1. Database Schema
   - We have the `candidate_skills` table for storing skills with preference values
   - We have the `candidate_preferences` table for storing work preferences

2. Handlers
   - `internal/handlers/skills.go` is extracting and storing skills from resume/interview
   - `internal/handlers/preferences.go` is extracting preferences from interview
   - `internal/handlers/workplace.go` is extracting and storing workplace preferences

3. UI Implementation
   - Basic template structure is in place with a single "Technical Skills" slider
   - Need to replace "Technical Skills" with individual skill sliders
   - Need to remove experience level sliders

## Next Steps

### 1. Update Skills UI

1. Remove the generic "Technical Skills" slider
2. Add individual sliders for each extracted skill from the database:
   ```html
   {{range .Skills}}
   <div class="grid">
       <label for="skill_{{.ID}}">
           {{.Name}} ({{.Level}} - {{.YearsUsed}} years)
           <span class="confidence">({{.Confidence}} confidence)</span>
       </label>
       <div class="slider-container">
           <input type="range" 
                  id="skill_{{.ID}}"
                  name="skill[{{.ID}}]"
                  min="1" 
                  max="5" 
                  value="{{.PreferenceValue}}"
                  data-skill-name="{{.Name}}">
           <div class="slider-labels">
               <span>Do Not Want</span>
               <span>Must Have</span>
           </div>
       </div>
       <label class="checkbox">
           <input type="checkbox" 
                  name="skill_search[{{.ID}}]"
                  {{if .IsSearchTerm}}checked{{end}}>
           Use in Search
       </label>
   </div>
   {{end}}
   ```

3. Remove all "Experience Level" sliders as these will be determined by the LLM
4. Add debug logging for skill preference updates:
   ```go
   logger.Debug("updating skill preference",
       slog.String("skill_id", skillID),
       slog.String("skill_name", skillName),
       slog.Int("preference_value", value),
       slog.Bool("is_search_term", isSearchTerm))
   ```

### 2. Update Skills Handler

Be sure to implement web request handlers off the application struct. Review other cmd/web handlers.*.go files for examples.

1. Add method to fetch all skills with preferences:
   ```go
   func (app *application) GetSkillsWithPreferences(ctx context.Context, candidateID string) ([]Skill, error) {
       app.logger.Debug("fetching skills with preferences",
           slog.String("candidate_id", candidateID))

       var skills []Skill
       err := app.db.SelectContext(ctx, &skills, `
           SELECT 
               s.*,
               COALESCE(p.value, 3) as preference_value,
               COALESCE(p.is_search_term, false) as is_search_term
           FROM candidate_skills s
           LEFT JOIN candidate_preferences p ON 
               p.candidate_id = s.candidate_id 
               AND p.category = 'skill'
               AND p.name = s.name
           WHERE s.candidate_id = $1
           ORDER BY s.confidence DESC, s.years_used DESC`,
           candidateID)
       
       return skills, err
   }
   ```

2. Add tests for the new functionality:
   ```go
   func TestSkillsHandler_GetSkillsWithPreferences(t *testing.T) {
       tests := []struct {
           name        string
           candidateID string
           wantSkills  []Skill
           wantErr     bool
       }{
           {
               name: "valid skills fetch",
               candidateID: "test-id",
               wantSkills: []Skill{
                   {
                       Name: "Golang",
                       Level: "Expert",
                       YearsUsed: 5,
                       PreferenceValue: 3,
                   },
               },
               wantErr: false,
           },
           // ... more test cases
       }

       for _, tt := range tests {
           t.Run(tt.name, func(t *testing.T) {
               // Test implementation
           })
       }
   }
   ```



### 4. Update Work Location UI

1. Update the work location slider to use 1-5 scale:
   ```html
   <fieldset>
       <legend>Work Location</legend>
       <p>Select your preferred work location type</p>
       
       <div class="grid">
           <label for="work_location">Work Location Preference</label>
           <div class="slider-container">
               <input type="range" 
                      id="work_location"
                      name="preference[work_location]"
                      min="1" 
                      max="5" 
                      value="{{.Preferences.WorkLocation}}">
               <div class="slider-labels">
                   <span>Fully Remote</span>
                   <span>Fully On Site</span>
               </div>
           </div>
       </div>
   </fieldset>
   ```

### 5. Update Search Integration

1. Update the search criteria generation to use the new workplace preference scale:
   ```go
   // Handle work location preference specially
   if loc := findWorkLocationPreference(preferences); loc != nil {
       switch loc.Value {
       case 1:
           criteria.WorkType = "remote"
       case 2:
           criteria.WorkType = "mostly_remote"
       case 3:
           criteria.WorkType = "hybrid"
       case 4:
           criteria.WorkType = "mostly_onsite"
       case 5:
           criteria.WorkType = "onsite"
       }
   }
   ```

### 6. Testing Plan

Only use the standard library and the "got/want" pattern for testing.

1. Unit Tests:
   - Test workplace preference validation
   - Test search criteria generation with new workplace scale

2. Integration Tests:
   - Test workplace preference extraction from interview
   - Test workplace preference storage flow
   - Test search integration with workplace preferences

3. End-to-End Tests:
   - Test complete flow from preference setting to job search
   - Test UI interactions with workplace preferences

### 7. Logging Strategy

Use the slog.With() function to add context to the logs.

Add detailed logging throughout the new workplace handler:

```go
logger.Info("storing workplace preference",
    slog.String("candidate_id", candidateID),
    slog.String("type", workplaceType),
    slog.Int("value", value))

logger.Info("retrieving workplace preferences",
    slog.String("candidate_id", candidateID))

logger.Info("building search criteria from workplace preferences",
    slog.String("candidate_id", candidateID),
    slog.String("work_type", workType))
```
