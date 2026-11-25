package schema

// SchemaV1 is the current schema version.
const SchemaV1 = "resume/v1"

// Kind identifies the type of YAML document.
type Kind string

const (
	KindProfile  Kind = "profile"
	KindBase     Kind = "base"
	KindResume   Kind = "resume"
	KindTemplate Kind = "template"
)

// Valid reports whether the kind is a recognized document kind.
func (k Kind) Valid() bool {
	switch k {
	case KindProfile, KindBase, KindResume, KindTemplate:
		return true
	}
	return false
}

// Envelope is the common header present in every YAML source file.
type Envelope struct {
	Schema string `yaml:"schema"`
	Kind   Kind   `yaml:"kind"`
	ID     string `yaml:"id"`
}

// Validate checks that the envelope has the required fields.
func (e Envelope) Validate() error {
	if e.Schema == "" {
		return &EnvelopeError{field: "schema", message: "schema is required"}
	}
	if e.Kind == "" {
		return &EnvelopeError{field: "kind", message: "kind is required"}
	}
	if !e.Kind.Valid() {
		return &EnvelopeError{field: "kind", message: "unknown kind: " + string(e.Kind)}
	}
	if e.ID == "" {
		return &EnvelopeError{field: "id", message: "id is required"}
	}
	return nil
}

// EnvelopeError describes a missing or invalid envelope field.
type EnvelopeError struct {
	field   string
	message string
}

func (e *EnvelopeError) Error() string {
	return e.message
}

// Field returns the envelope field that caused the error.
func (e *EnvelopeError) Field() string {
	return e.field
}

// ---- Profile ----

// Profile contains reusable career information.
type Profile struct {
	Envelope   Envelope          `yaml:"-"`
	Header     Header            `yaml:"header"`
	Employment []Employer        `yaml:"employment,omitempty"`
	Skills     []SkillGroup      `yaml:"skills,omitempty"`
	Projects   []Project         `yaml:"projects,omitempty"`
	Education  []Education       `yaml:"education,omitempty"`
	Certs      []Certification   `yaml:"certifications,omitempty"`
	Summaries  []Summary         `yaml:"summaries,omitempty"`
	Impact     []ImpactStatement `yaml:"impact,omitempty"`
}

// Header contains contact information.
type Header struct {
	Name        string   `yaml:"name"`
	Phone       string   `yaml:"phone,omitempty"`
	Email       string   `yaml:"email,omitempty"`
	Location    string   `yaml:"location,omitempty"`
	GitHub      string   `yaml:"github,omitempty"`
	LinkedIn    string   `yaml:"linkedin,omitempty"`
	Credentials []string `yaml:"credentials,omitempty"`
	Portfolio   []Link   `yaml:"portfolio,omitempty"`
}

// Link is a labeled URL.
type Link struct {
	Label string `yaml:"label"`
	URL   string `yaml:"url"`
}

// Employer represents a company with one or more roles.
type Employer struct {
	ID       string `yaml:"id"`
	Name     string `yaml:"name"`
	Location string `yaml:"location,omitempty"`
	Link     string `yaml:"link,omitempty"`
	Roles    []Role `yaml:"roles"`
}

// Role represents a position within an employer.
type Role struct {
	ID              string   `yaml:"id"`
	Title           string   `yaml:"title"`
	Start           string   `yaml:"start"`
	End             string   `yaml:"end,omitempty"`
	Accomplishments []string `yaml:"accomplishments,omitempty"`
}

// SkillGroup is a named collection of skills.
type SkillGroup struct {
	ID     string   `yaml:"id"`
	Name   string   `yaml:"name"`
	Skills []string `yaml:"skills"`
}

// Project represents a project or open-source work.
type Project struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	Role            string   `yaml:"role,omitempty"`
	Start           string   `yaml:"start,omitempty"`
	End             string   `yaml:"end,omitempty"`
	Link            string   `yaml:"link,omitempty"`
	Tech            string   `yaml:"tech,omitempty"`
	Accomplishments []string `yaml:"accomplishments,omitempty"`
}

// Education represents an academic credential.
type Education struct {
	ID          string `yaml:"id"`
	Institution string `yaml:"institution"`
	Degree      string `yaml:"degree"`
	Field       string `yaml:"field,omitempty"`
	Graduation  string `yaml:"graduation,omitempty"`
	Honors      string `yaml:"honors,omitempty"`
}

// Certification represents a professional certification.
type Certification struct {
	ID            string `yaml:"id"`
	Name          string `yaml:"name"`
	Issuer        string `yaml:"issuer"`
	DateEarned    string `yaml:"dateEarned,omitempty"`
	Expiration    string `yaml:"expiration,omitempty"`
	CredentialURL string `yaml:"credentialUrl,omitempty"`
}

// Summary is a reusable, identified summary block.
type Summary struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// ImpactStatement is a reusable, identified impact statement.
type ImpactStatement struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// ---- Base ----

// Base contains selections and settings shared by multiple résumés.
type Base struct {
	Envelope   Envelope         `yaml:"-"`
	Profile    string           `yaml:"profile,omitempty"`
	Template   string           `yaml:"template,omitempty"`
	PageLimit  int              `yaml:"pageLimit,omitempty"`
	Sections   []string         `yaml:"sections,omitempty"`
	Employment []Employer       `yaml:"employment,omitempty"`
	Education  []BaseRef        `yaml:"education,omitempty"`
	Certs      []BaseRef        `yaml:"certifications,omitempty"`
	Skills     []SkillGroup     `yaml:"skills,omitempty"`
	Projects   []BaseRef        `yaml:"projects,omitempty"`
	Visibility VisibilityConfig `yaml:"visibility,omitempty"`
}

// BaseRef references a profile item by ID.
type BaseRef struct {
	ID string `yaml:"id"`
}

// VisibilityConfig controls default visibility of optional sections.
type VisibilityConfig struct {
	Education      *bool `yaml:"education,omitempty"`
	Certifications *bool `yaml:"certifications,omitempty"`
}

// ---- Resume ----

// Resume contains the target-specific résumé definition.
type Resume struct {
	Envelope   Envelope    `yaml:"-"`
	Target     Target      `yaml:"target"`
	Profile    string      `yaml:"profile,omitempty"`
	Base       string      `yaml:"base,omitempty"`
	Template   string      `yaml:"template,omitempty"`
	Output     string      `yaml:"output,omitempty"`
	Headline   string      `yaml:"headline,omitempty"`
	Summary    *SummarySel `yaml:"summary,omitempty"`
	Impact     []ImpactSel `yaml:"impact,omitempty"`
	Skills     []SkillSel  `yaml:"skills,omitempty"`
	Experience []ExpSel    `yaml:"experience,omitempty"`
	Projects   []Ref       `yaml:"projects,omitempty"`
	Education  []Ref       `yaml:"education,omitempty"`
	Certs      []Ref       `yaml:"certifications,omitempty"`
	Sections   []string    `yaml:"sections,omitempty"`
	PageLimit  int         `yaml:"pageLimit,omitempty"`
	Metadata   PDFMeta     `yaml:"metadata,omitempty"`
}

// Target identifies the job this résumé is for.
type Target struct {
	Company string `yaml:"company"`
	Role    string `yaml:"role"`
	URL     string `yaml:"url,omitempty"`
	Notes   string `yaml:"notes,omitempty"`
}

// SummarySel selects a summary by ID or provides inline text.
type SummarySel struct {
	ID   string `yaml:"id,omitempty"`
	Text string `yaml:"text,omitempty"`
}

// ImpactSel selects an impact statement by ID or provides inline text.
type ImpactSel struct {
	ID   string `yaml:"id,omitempty"`
	Text string `yaml:"text,omitempty"`
}

// SkillSel selects a skill group by ID with optional skill filtering.
type SkillSel struct {
	Group  string   `yaml:"group"`
	Skills []string `yaml:"skills,omitempty"`
}

// ExpSel selects experience items by ID.
type ExpSel struct {
	Company string   `yaml:"company"`
	Roles   []string `yaml:"roles,omitempty"`
	Bullets []string `yaml:"bullets,omitempty"`
}

// Ref is a simple reference to a profile item by ID.
type Ref struct {
	ID string `yaml:"id"`
}

// PDFMeta holds optional PDF document properties.
type PDFMeta struct {
	Title   string `yaml:"title,omitempty"`
	Author  string `yaml:"author,omitempty"`
	Subject string `yaml:"subject,omitempty"`
}

// ---- Template ----

// Template contains presentation rules only.
type Template struct {
	Envelope   Envelope        `yaml:"-"`
	Page       PageConfig      `yaml:"page"`
	Margins    MarginConfig    `yaml:"margins"`
	Fonts      FontConfig      `yaml:"fonts"`
	Colors     ColorConfig     `yaml:"colors"`
	Sizes      SizeConfig      `yaml:"sizes"`
	Spacing    SpacingConfig   `yaml:"spacing"`
	Headings   HeadingConfig   `yaml:"headings"`
	Indent     IndentConfig    `yaml:"indent"`
	Header     HeaderLayout    `yaml:"headerLayout"`
	Footer     FooterLayout    `yaml:"footerLayout"`
	PageBreaks PageBreakConfig `yaml:"pageBreaks"`
}

// PageConfig defines page dimensions.
type PageConfig struct {
	Width  float64 `yaml:"width"`
	Height float64 `yaml:"height"`
}

// MarginConfig defines page margins.
type MarginConfig struct {
	Top    float64 `yaml:"top"`
	Bottom float64 `yaml:"bottom"`
	Left   float64 `yaml:"left"`
	Right  float64 `yaml:"right"`
}

// FontConfig defines font families.
type FontConfig struct {
	Regular string `yaml:"regular"`
	Bold    string `yaml:"bold"`
	Italic  string `yaml:"italic,omitempty"`
	BoldIt  string `yaml:"boldItalic,omitempty"`
}

// ColorConfig defines colors used in the template.
type ColorConfig struct {
	Text       string `yaml:"text"`
	Heading    string `yaml:"heading"`
	Link       string `yaml:"link,omitempty"`
	Rule       string `yaml:"rule,omitempty"`
	Background string `yaml:"background,omitempty"`
}

// SizeConfig defines font sizes in points.
type SizeConfig struct {
	Name      float64 `yaml:"name"`
	Contact   float64 `yaml:"contact"`
	Heading   float64 `yaml:"heading"`
	RoleTitle float64 `yaml:"roleTitle"`
	Body      float64 `yaml:"body"`
	Footer    float64 `yaml:"footer"`
}

// SpacingConfig defines spacing values.
type SpacingConfig struct {
	Line      float64 `yaml:"line"`
	Section   float64 `yaml:"section"`
	EntryGap  float64 `yaml:"entryGap"`
	ImpactGap float64 `yaml:"impactGap"`
	RoleGap   float64 `yaml:"roleGap"`
	BulletGap float64 `yaml:"bulletGap"`
}

// HeadingConfig defines section heading style.
type HeadingConfig struct {
	Uppercase *bool   `yaml:"uppercase,omitempty"`
	LetterSp  string  `yaml:"letterSpacing,omitempty"`
	Rule      *bool   `yaml:"rule,omitempty"`
	RuleSize  float64 `yaml:"ruleSize,omitempty"`
}

// IndentConfig defines indentation values.
type IndentConfig struct {
	Bullet float64 `yaml:"bullet"`
	Role   float64 `yaml:"role"`
	Accomp float64 `yaml:"accomplishment"`
}

// HeaderLayout defines the header section layout.
type HeaderLayout struct {
	Align   string `yaml:"align,omitempty"`
	Compact *bool  `yaml:"compact,omitempty"`
}

// FooterLayout defines the footer layout.
type FooterLayout struct {
	Name *bool `yaml:"name,omitempty"`
	Page *bool `yaml:"page,omitempty"`
}

// PageBreakConfig defines page-break behavior.
type PageBreakConfig struct {
	KeepHeadingWithContent *bool `yaml:"keepHeadingWithContent,omitempty"`
	KeepRoleWithAccomp     *bool `yaml:"keepRoleWithAccomplishment,omitempty"`
	PageLimit              int   `yaml:"pageLimit,omitempty"`
}
