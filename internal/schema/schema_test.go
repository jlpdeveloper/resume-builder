package schema

import (
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

// roundTrip marshals a value to YAML and unmarshals it back, then compares.
func roundTrip[T any](t *testing.T, original T) T {
	t.Helper()
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded T
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Marshal both to YAML for comparison (avoids pointer comparison issues).
	origData, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("re-marshal original failed: %v", err)
	}
	decData, err := yaml.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-marshal decoded failed: %v", err)
	}

	if string(origData) != string(decData) {
		t.Errorf("round-trip mismatch\noriginal:\n%s\ndecoded:\n%s", origData, decData)
	}

	return decoded
}

func TestEnvelope_Validate(t *testing.T) {
	tests := []struct {
		name    string
		env     Envelope
		wantErr bool
		field   string
	}{
		{
			name:    "valid",
			env:     Envelope{Schema: SchemaV1, Kind: KindProfile, ID: "test"},
			wantErr: false,
		},
		{
			name:    "missing schema",
			env:     Envelope{Kind: KindProfile, ID: "test"},
			wantErr: true,
			field:   "schema",
		},
		{
			name:    "missing kind",
			env:     Envelope{Schema: SchemaV1, ID: "test"},
			wantErr: true,
			field:   "kind",
		},
		{
			name:    "invalid kind",
			env:     Envelope{Schema: SchemaV1, Kind: "unknown", ID: "test"},
			wantErr: true,
			field:   "kind",
		},
		{
			name:    "missing id",
			env:     Envelope{Schema: SchemaV1, Kind: KindProfile},
			wantErr: true,
			field:   "id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.env.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				var envErr *EnvelopeError
				if !errors.As(err, &envErr) {
					t.Errorf("expected *EnvelopeError, got %T", err)
					return
				}
				if envErr.Field() != tt.field {
					t.Errorf("expected field %q, got %q", tt.field, envErr.Field())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

func TestKind_Valid(t *testing.T) {
	tests := []struct {
		kind Kind
		want bool
	}{
		{KindProfile, true},
		{KindBase, true},
		{KindResume, true},
		{KindTemplate, true},
		{Kind("unknown"), false},
		{Kind(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			got := tt.kind.Valid()
			if got != tt.want {
				t.Errorf("Kind(%q).Valid() = %v, want %v", tt.kind, got, tt.want)
			}
		})
	}
}

func TestProfile_RoundTrip(t *testing.T) {
	profile := Profile{
		Envelope: Envelope{Schema: SchemaV1, Kind: KindProfile, ID: "josh-potts"},
		Header: Header{
			Name:     "Josh Potts",
			Phone:    "555-0100",
			Email:    "josh@example.com",
			Location: "Austin, TX",
			GitHub:   "github.com/josh",
			LinkedIn: "linkedin.com/in/josh",
			Credentials: []string{
				"AWS Solutions Architect",
			},
		},
		Employment: []Employer{
			{
				ID:   "acme",
				Name: "Acme Corp",
				Roles: []Role{
					{
						ID:    "senior-eng",
						Title: "Senior Engineer",
						Start: "2022-01",
						Accomplishments: []string{
							"Led migration to microservices",
						},
					},
				},
			},
		},
		Skills: []SkillGroup{
			{
				ID:     "languages",
				Name:   "Languages",
				Skills: []string{"Go", "Python", "TypeScript"},
			},
		},
		Projects: []Project{
			{
				ID:   "open-source-tool",
				Name: "Open Source Tool",
				Role: "Maintainer",
				Link: "github.com/josh/tool",
			},
		},
		Education: []Education{
			{
				ID:          "bs-cs",
				Institution: "State University",
				Degree:      "BS",
				Field:       "Computer Science",
				Graduation:  "2018-05",
			},
		},
		Certs: []Certification{
			{
				ID:         "aws-saa",
				Name:       "AWS Solutions Architect",
				Issuer:     "Amazon",
				DateEarned: "2023-03",
			},
		},
		Summaries: []Summary{
			{
				ID:   "general",
				Text: "Senior engineer with 5+ years of experience.",
			},
		},
		Impact: []ImpactStatement{
			{
				ID:   "perf",
				Text: "Improved system performance by 40%.",
			},
		},
	}

	roundTrip(t, profile)
}

func TestBase_RoundTrip(t *testing.T) {
	base := Base{
		Envelope:  Envelope{Schema: SchemaV1, Kind: KindBase, ID: "staff-engineer"},
		Profile:   "profile.yml",
		Template:  "templates/standard.yml",
		PageLimit: 2,
		Sections:  []string{"summary", "experience", "skills", "education"},
		Education: []BaseRef{
			{ID: "bs-cs"},
		},
		Certs: []BaseRef{
			{ID: "aws-saa"},
		},
		Skills: []SkillGroup{
			{
				ID:     "languages",
				Name:   "Languages",
				Skills: []string{"Go", "Python"},
			},
		},
		Projects: []BaseRef{
			{ID: "open-source-tool"},
		},
	}

	roundTrip(t, base)
}

func TestResume_RoundTrip(t *testing.T) {
	resume := Resume{
		Envelope: Envelope{Schema: SchemaV1, Kind: KindResume, ID: "acme-apply"},
		Target: Target{
			Company: "Acme Corp",
			Role:    "Senior Engineer",
			URL:     "https://acme.com/jobs/123",
		},
		Base:     "bases/staff-engineer.yml",
		Template: "templates/standard.yml",
		Output:   "output/acme-senior-engineer.pdf",
		Headline: "Senior Software Engineer",
		Summary: &SummarySel{
			ID: "general",
		},
		Impact: []ImpactSel{
			{ID: "perf"},
			{Text: "Reduced deployment time from hours to minutes."},
		},
		Skills: []SkillSel{
			{Group: "languages"},
		},
		Experience: []ExpSel{
			{
				Company: "acme",
				Roles:   []string{"senior-eng"},
				Bullets: []string{"Led migration to microservices"},
			},
		},
		Projects: []Ref{
			{ID: "open-source-tool"},
		},
		Education: []Ref{
			{ID: "bs-cs"},
		},
		Certs: []Ref{
			{ID: "aws-saa"},
		},
		Sections:  []string{"summary", "impact", "experience", "skills", "projects", "education"},
		PageLimit: 2,
		Metadata: PDFMeta{
			Title:   "Josh Potts - Senior Engineer",
			Author:  "Josh Potts",
			Subject: "Resume",
		},
	}

	roundTrip(t, resume)
}

func TestTemplate_RoundTrip(t *testing.T) {
	trueVal := true
	template := Template{
		Envelope: Envelope{Schema: SchemaV1, Kind: KindTemplate, ID: "standard"},
		Page:     PageConfig{Width: 8.5, Height: 11},
		Margins:  MarginConfig{Top: 0.5, Bottom: 0.5, Left: 0.75, Right: 0.75},
		Fonts: FontConfig{
			Regular: "Helvetica",
			Bold:    "Helvetica-Bold",
			Italic:  "Helvetica-Oblique",
		},
		Colors: ColorConfig{
			Text:    "#333333",
			Heading: "#1a73e8",
			Link:    "#1a73e8",
			Rule:    "#cccccc",
		},
		Sizes: SizeConfig{
			Name:      16,
			Contact:   9,
			Heading:   11,
			RoleTitle: 10,
			Body:      9,
			Footer:    8,
		},
		Spacing: SpacingConfig{
			Line:      1.2,
			Section:   6,
			RoleGap:   4,
			BulletGap: 2,
		},
		Headings: HeadingConfig{
			Uppercase: &trueVal,
			Rule:      &trueVal,
			RuleSize:  0.5,
		},
		Indent: IndentConfig{
			Bullet: 0.25,
			Role:   0.25,
			Accomp: 0.5,
		},
		Header: HeaderLayout{
			Align:   "center",
			Compact: &trueVal,
		},
		Footer: FooterLayout{
			Name: &trueVal,
			Page: &trueVal,
		},
		PageBreaks: PageBreakConfig{
			KeepHeadingWithContent: &trueVal,
			KeepRoleWithAccomp:     &trueVal,
		},
	}

	roundTrip(t, template)
}

func TestEnvelope_YAMLRoundTrip(t *testing.T) {
	env := Envelope{
		Schema: SchemaV1,
		Kind:   KindResume,
		ID:     "test-resume",
	}

	data, err := yaml.Marshal(env)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded Envelope
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Schema != env.Schema {
		t.Errorf("schema: got %q, want %q", decoded.Schema, env.Schema)
	}
	if decoded.Kind != env.Kind {
		t.Errorf("kind: got %q, want %q", decoded.Kind, env.Kind)
	}
	if decoded.ID != env.ID {
		t.Errorf("id: got %q, want %q", decoded.ID, env.ID)
	}

	if err := decoded.Validate(); err != nil {
		t.Errorf("decoded envelope should be valid: %v", err)
	}
}
