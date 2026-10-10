package internal

import (
	"strings"
	"testing"
)

func TestBuildPersonalSteeringQueryWithoutAvoidedTraits(t *testing.T) {
	resume := Resume{RawText: "Built distributed Go services and managed platform teams."}
	preferences := PersonalPreferences{
		TargetRoles: []string{"Platform Engineering Manager"},
		Locations:   "Phoenix, Arizona", WorkTypes: []WorkType{WorkTypeRemote},
		Seniority: ExperienceLevelDirector, Timezone: "America/Phoenix",
		DesiredTraits: "hands-on technical leadership",
	}
	query, err := BuildPersonalSteeringQuery(resume, preferences)
	if err != nil {
		t.Fatalf("BuildPersonalSteeringQuery() error = %v", err)
	}
	if query.MoveAway != "" || query.ForceAway != 0 {
		t.Errorf("empty avoided traits yielded MoveAway = %q, ForceAway = %v", query.MoveAway, query.ForceAway)
	}
	if !strings.Contains(query.MoveToward, resume.RawText) {
		t.Errorf("MoveToward = %q does not include resume context", query.MoveToward)
	}
	if err := query.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestRegenerateProfileStatement(t *testing.T) {
	annotation := "Led a platform team."
	version := UserStatementVersion{
		GeneratedBy: "profile",
		SourceData: map[string]any{
			"source_type": "profile", "resume_raw_text": "Built Go services.",
			"resume_annotated": &annotation,
			"preferences": map[string]any{
				"target_roles": []string{"Staff Engineer"},
				"locations":    "Phoenix", "work_types": []WorkType{WorkTypeRemote},
				"seniority":      string(ExperienceLevelMidSeniorLevel),
				"desired_traits": "small teams", "avoided_traits": "", "timezone": "America/Phoenix",
			},
		},
	}
	statement, err := RegenerateProfileStatement(version)
	if err != nil {
		t.Fatalf("RegenerateProfileStatement() error = %v", err)
	}
	for _, source := range []string{"Staff Engineer", "Phoenix", "small teams", annotation} {
		if !strings.Contains(statement, source) {
			t.Errorf("statement %q omits source %q", statement, source)
		}
	}
}
