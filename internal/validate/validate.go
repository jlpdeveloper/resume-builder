package validate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"gopkg.in/yaml.v3"
)

// Diagnostic describes a validation error or warning.
type Diagnostic struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

// Result contains validation errors and non-fatal warnings.
type Result struct {
	Valid    bool         `json:"valid"`
	Errors   []Diagnostic `json:"errors"`
	Warnings []Diagnostic `json:"warnings"`
}

type parsedFiles struct {
	profiles  map[string]*schema.Profile
	bases     map[string]*schema.Base
	resumes   map[string]*schema.Resume
	templates map[string]*schema.Template
}

type profileFile struct {
	schema.Envelope `yaml:",inline"`
	schema.Profile  `yaml:",inline"`
}

type baseFile struct {
	schema.Envelope `yaml:",inline"`
	schema.Base     `yaml:",inline"`
}

type resumeFile struct {
	schema.Envelope `yaml:",inline"`
	schema.Resume   `yaml:",inline"`
}

type templateFile struct {
	schema.Envelope `yaml:",inline"`
	schema.Template `yaml:",inline"`
}

var (
	errorLine  = regexp.MustCompile(`line (\d+)`)
	errorField = regexp.MustCompile(`field ([^ ]+) not found`)
)

// Validate checks a YAML document and every file and content reference it uses.
func Validate(path string) Result {
	graph, err := (&resolve.Resolver{}).Resolve(path)
	if err != nil {
		file := path
		var located interface{ FilePath() string }
		if ok := errorAs(err, &located); ok && located.FilePath() != "" {
			file = located.FilePath()
		}
		return Result{
			Valid:    false,
			Errors:   []Diagnostic{{Path: file, Line: lineInError(err), Field: fieldInError(err), Message: err.Error()}},
			Warnings: []Diagnostic{},
		}
	}

	result := Result{Valid: true, Errors: []Diagnostic{}, Warnings: []Diagnostic{}}
	files := parsedFiles{
		profiles:  make(map[string]*schema.Profile),
		bases:     make(map[string]*schema.Base),
		resumes:   make(map[string]*schema.Resume),
		templates: make(map[string]*schema.Template),
	}
	for _, doc := range graph.Documents {
		value, err := decodeStrict(doc)
		if err != nil {
			field := ""
			if match := errorField.FindStringSubmatch(err.Error()); len(match) == 2 {
				field = match[1]
			}
			result.Errors = append(result.Errors, Diagnostic{
				Path:    doc.Path,
				Line:    lineInError(err),
				Field:   field,
				Message: err.Error(),
			})
			continue
		}
		switch doc.Kind {
		case schema.KindProfile:
			profile := value.(*schema.Profile)
			files.profiles[doc.Path] = profile
			validateProfile(&result, doc, profile)
		case schema.KindBase:
			base := value.(*schema.Base)
			files.bases[doc.Path] = base
			validateBase(&result, doc, base)
		case schema.KindResume:
			resume := value.(*schema.Resume)
			files.resumes[doc.Path] = resume
			validateResume(&result, doc, resume)
		case schema.KindTemplate:
			template := value.(*schema.Template)
			files.templates[doc.Path] = template
			validateTemplate(&result, doc, template)
		}
	}

	validateReferences(&result, graph.Documents, files)
	result.Valid = len(result.Errors) == 0
	return result
}

// Run validates a single path and writes either human-readable or JSON output.
func Run(args []string, stdout, stderr io.Writer, jsonOutput bool) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(stderr, "Usage: resume validate PATH")
		return 2
	}

	result := Validate(args[0])
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "write validation output: %v\n", err)
			return 1
		}
	} else {
		writeHuman(stdout, args[0], result)
	}
	if !result.Valid {
		return 1
	}
	return 0
}

func decodeStrict(doc *resolve.Document) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(doc.Raw))
	decoder.KnownFields(true)
	switch doc.Kind {
	case schema.KindProfile:
		var file profileFile
		if err := decoder.Decode(&file); err != nil {
			return nil, err
		}
		return &file.Profile, nil
	case schema.KindBase:
		var file baseFile
		if err := decoder.Decode(&file); err != nil {
			return nil, err
		}
		return &file.Base, nil
	case schema.KindResume:
		var file resumeFile
		if err := decoder.Decode(&file); err != nil {
			return nil, err
		}
		return &file.Resume, nil
	case schema.KindTemplate:
		var file templateFile
		if err := decoder.Decode(&file); err != nil {
			return nil, err
		}
		return &file.Template, nil
	default:
		return nil, fmt.Errorf("unsupported document kind %q", doc.Kind)
	}
}

func validateProfile(result *Result, doc *resolve.Document, profile *schema.Profile) {
	require(result, doc, "header.name", profile.Header.Name)
	checkURL(result, doc, "header.github", profile.Header.GitHub)
	checkURL(result, doc, "header.linkedin", profile.Header.LinkedIn)
	for i, link := range profile.Header.Portfolio {
		checkURL(result, doc, fmt.Sprintf("header.portfolio[%d].url", i), link.URL)
	}

	seenEmployers := make(map[string]bool)
	seenRoles := make(map[string]bool)
	seenBullets := make(map[string]bool)
	for i, employer := range profile.Employment {
		prefix := fmt.Sprintf("employment[%d]", i)
		require(result, doc, prefix+".id", employer.ID)
		require(result, doc, prefix+".name", employer.Name)
		duplicateID(result, doc, prefix+".id", employer.ID, seenEmployers)
		checkURL(result, doc, prefix+".link", employer.Link)
		for j, role := range employer.Roles {
			field := fmt.Sprintf("%s.roles[%d]", prefix, j)
			require(result, doc, field+".id", role.ID)
			require(result, doc, field+".title", role.Title)
			checkDate(result, doc, field+".start", role.Start, false)
			checkDate(result, doc, field+".end", role.End, true)
			duplicateID(result, doc, field+".id", role.ID, seenRoles)
			for _, bullet := range role.Accomplishments {
				key := strings.ToLower(strings.TrimSpace(bullet))
				if key == "" {
					continue
				}
				if seenBullets[key] {
					warning(result, doc, field+".accomplishments", "duplicate accomplishment text")
				}
				seenBullets[key] = true
			}
		}
	}

	validateIDs(result, doc, "skills", profile.Skills, func(item schema.SkillGroup) string { return item.ID })
	for i, group := range profile.Skills {
		require(result, doc, fmt.Sprintf("skills[%d].name", i), group.Name)
	}
	validateIDs(result, doc, "projects", profile.Projects, func(item schema.Project) string { return item.ID })
	for i, project := range profile.Projects {
		require(result, doc, fmt.Sprintf("projects[%d].name", i), project.Name)
		checkURL(result, doc, fmt.Sprintf("projects[%d].link", i), project.Link)
		checkDate(result, doc, fmt.Sprintf("projects[%d].start", i), project.Start, true)
		checkDate(result, doc, fmt.Sprintf("projects[%d].end", i), project.End, true)
	}
	validateIDs(result, doc, "education", profile.Education, func(item schema.Education) string { return item.ID })
	for i, education := range profile.Education {
		require(result, doc, fmt.Sprintf("education[%d].institution", i), education.Institution)
		require(result, doc, fmt.Sprintf("education[%d].degree", i), education.Degree)
		checkDate(result, doc, fmt.Sprintf("education[%d].graduation", i), education.Graduation, true)
	}
	validateIDs(result, doc, "certifications", profile.Certs, func(item schema.Certification) string { return item.ID })
	for i, cert := range profile.Certs {
		require(result, doc, fmt.Sprintf("certifications[%d].name", i), cert.Name)
		require(result, doc, fmt.Sprintf("certifications[%d].issuer", i), cert.Issuer)
		checkDate(result, doc, fmt.Sprintf("certifications[%d].dateEarned", i), cert.DateEarned, true)
		checkDate(result, doc, fmt.Sprintf("certifications[%d].expiration", i), cert.Expiration, true)
		checkURL(result, doc, fmt.Sprintf("certifications[%d].credentialUrl", i), cert.CredentialURL)
	}
	validateIDs(result, doc, "summaries", profile.Summaries, func(item schema.Summary) string { return item.ID })
	for i, summary := range profile.Summaries {
		require(result, doc, fmt.Sprintf("summaries[%d].text", i), summary.Text)
	}
	validateIDs(result, doc, "impact", profile.Impact, func(item schema.ImpactStatement) string { return item.ID })
	for i, impact := range profile.Impact {
		require(result, doc, fmt.Sprintf("impact[%d].text", i), impact.Text)
	}
}

func validateBase(result *Result, doc *resolve.Document, base *schema.Base) {
	if base.PageLimit < 0 {
		addError(result, doc, "pageLimit", "page limit cannot be negative")
	}
	validateSections(result, doc, base.Sections)
	validateIDs(result, doc, "employment", base.Employment, func(item schema.Employer) string { return item.ID })
	validateIDs(result, doc, "skills", base.Skills, func(item schema.SkillGroup) string { return item.ID })
	for i, group := range base.Skills {
		require(result, doc, fmt.Sprintf("skills[%d].name", i), group.Name)
	}
}

func validateResume(result *Result, doc *resolve.Document, resume *schema.Resume) {
	require(result, doc, "target.company", resume.Target.Company)
	require(result, doc, "target.role", resume.Target.Role)
	checkURL(result, doc, "target.url", resume.Target.URL)
	if resume.PageLimit < 0 {
		addError(result, doc, "pageLimit", "page limit cannot be negative")
	}
	if resume.Summary != nil && resume.Summary.ID != "" && resume.Summary.Text != "" {
		addError(result, doc, "summary", "select a summary by id or provide inline text, not both")
	} else if resume.Summary != nil && resume.Summary.ID == "" && strings.TrimSpace(resume.Summary.Text) == "" {
		addError(result, doc, "summary", "summary must include an id or inline text")
	}
	for i, impact := range resume.Impact {
		if (impact.ID == "") == (strings.TrimSpace(impact.Text) == "") {
			addError(result, doc, fmt.Sprintf("impact[%d]", i), "impact must include either an id or inline text")
		}
	}
	validateSections(result, doc, resume.Sections)
}

func validateTemplate(result *Result, doc *resolve.Document, template *schema.Template) {
	if template.Page.Width <= 0 {
		addError(result, doc, "page.width", "page width must be greater than zero")
	}
	if template.Page.Height <= 0 {
		addError(result, doc, "page.height", "page height must be greater than zero")
	}
	if template.Margins.Top < 0 || template.Margins.Bottom < 0 || template.Margins.Left < 0 || template.Margins.Right < 0 {
		addError(result, doc, "margins", "margins cannot be negative")
	}
}

func validateReferences(result *Result, docs []*resolve.Document, files parsedFiles) {
	for _, doc := range docs {
		switch doc.Kind {
		case schema.KindResume:
			resume := files.resumes[doc.Path]
			if resume == nil {
				continue
			}
			profile := profileFor(doc, docs, files)
			if profile == nil {
				continue
			}
			validateResumeReferences(result, doc, resume, profile)
			warnEmptySections(result, doc, resume, profile)
		case schema.KindBase:
			base := files.bases[doc.Path]
			if base == nil {
				continue
			}
			profile := profileFor(doc, docs, files)
			if profile != nil {
				validateBaseReferences(result, doc, base, profile)
			}
		}
	}
}

func profileFor(doc *resolve.Document, docs []*resolve.Document, files parsedFiles) *schema.Profile {
	profileRef := doc.Profile
	if doc.Kind == schema.KindResume && profileRef == "" {
		for _, other := range docs {
			if other.Kind == schema.KindBase {
				if profile := profileFor(other, docs, files); profile != nil {
					return profile
				}
			}
		}
	}
	if profileRef != "" {
		path := filepath.Clean(filepath.Join(filepath.Dir(doc.Path), profileRef))
		if profile := files.profiles[path]; profile != nil {
			return profile
		}
		for p, profile := range files.profiles {
			if filepath.Clean(p) == path {
				return profile
			}
		}
	}
	for _, profile := range files.profiles {
		return profile
	}
	return nil
}

func validateResumeReferences(result *Result, doc *resolve.Document, resume *schema.Resume, profile *schema.Profile) {
	if selection := resume.Summary; selection != nil && selection.ID != "" {
		found := false
		for _, summary := range profile.Summaries {
			found = found || summary.ID == selection.ID
		}
		if !found {
			addError(result, doc, "summary.id", fmt.Sprintf("unknown summary id %q", selection.ID))
		}
	}
	for i, selection := range resume.Impact {
		if selection.ID != "" && !hasImpact(profile, selection.ID) {
			addError(result, doc, fmt.Sprintf("impact[%d].id", i), fmt.Sprintf("unknown impact id %q", selection.ID))
		}
	}
	for i, selection := range resume.Skills {
		group, ok := findSkillGroup(profile.Skills, selection.Group)
		if !ok {
			addError(result, doc, fmt.Sprintf("skills[%d].group", i), fmt.Sprintf("unknown skill group %q", selection.Group))
			continue
		}
		for _, skill := range selection.Skills {
			if !contains(group.Skills, skill) {
				addError(result, doc, fmt.Sprintf("skills[%d].skills", i), fmt.Sprintf("unknown skill %q in group %q", skill, selection.Group))
			}
		}
	}
	for i, selection := range resume.Experience {
		employer, ok := findEmployer(profile.Employment, selection.Company)
		if !ok {
			addError(result, doc, fmt.Sprintf("experience[%d].company", i), fmt.Sprintf("unknown company %q", selection.Company))
			continue
		}
		var selectedRoles []schema.Role
		for _, roleID := range selection.Roles {
			found := false
			for _, role := range employer.Roles {
				if role.ID == roleID {
					selectedRoles = append(selectedRoles, role)
					found = true
				}
			}
			if !found {
				addError(result, doc, fmt.Sprintf("experience[%d].roles", i), fmt.Sprintf("unknown role id %q for company %q", roleID, selection.Company))
			}
		}
		if len(selection.Roles) == 0 {
			selectedRoles = employer.Roles
		}
		for _, bullet := range selection.Bullets {
			found := false
			for _, role := range selectedRoles {
				found = found || contains(role.Accomplishments, bullet)
			}
			if !found {
				addError(result, doc, fmt.Sprintf("experience[%d].bullets", i), "unknown accomplishment selection")
			}
		}
	}
	checkRefs(result, doc, "projects", resume.Projects, func(ref schema.Ref) string { return ref.ID }, func(ref schema.Ref) bool { return hasProject(profile, ref.ID) })
	checkRefs(result, doc, "education", resume.Education, func(ref schema.Ref) string { return ref.ID }, func(ref schema.Ref) bool { return hasEducation(profile, ref.ID) })
	checkRefs(result, doc, "certifications", resume.Certs, func(ref schema.Ref) string { return ref.ID }, func(ref schema.Ref) bool { return hasCert(profile, ref.ID) })
}

func validateBaseReferences(result *Result, doc *resolve.Document, base *schema.Base, profile *schema.Profile) {
	for i, employerSelection := range base.Employment {
		employer, ok := findEmployer(profile.Employment, employerSelection.ID)
		if !ok {
			addError(result, doc, fmt.Sprintf("employment[%d].id", i), fmt.Sprintf("unknown company id %q", employerSelection.ID))
			continue
		}
		for j, selectedRole := range employerSelection.Roles {
			found := false
			for _, role := range employer.Roles {
				found = found || role.ID == selectedRole.ID
			}
			if !found {
				addError(result, doc, fmt.Sprintf("employment[%d].roles[%d].id", i, j), fmt.Sprintf("unknown role id %q", selectedRole.ID))
			}
		}
	}
	checkRefs(result, doc, "projects", base.Projects, func(ref schema.BaseRef) string { return ref.ID }, func(ref schema.BaseRef) bool { return hasProject(profile, ref.ID) })
	checkRefs(result, doc, "education", base.Education, func(ref schema.BaseRef) string { return ref.ID }, func(ref schema.BaseRef) bool { return hasEducation(profile, ref.ID) })
	checkRefs(result, doc, "certifications", base.Certs, func(ref schema.BaseRef) string { return ref.ID }, func(ref schema.BaseRef) bool { return hasCert(profile, ref.ID) })
}

func warnEmptySections(result *Result, doc *resolve.Document, resume *schema.Resume, profile *schema.Profile) {
	for _, section := range resume.Sections {
		empty := false
		switch section {
		case "summary":
			empty = resume.Summary == nil
		case "impact":
			empty = len(resume.Impact) == 0
		case "experience":
			empty = len(resume.Experience) == 0
		case "skills":
			empty = len(resume.Skills) == 0
		case "projects":
			empty = len(resume.Projects) == 0 && len(profile.Projects) == 0
		case "education":
			empty = len(resume.Education) == 0 && len(profile.Education) == 0
		case "certifications":
			empty = len(resume.Certs) == 0 && len(profile.Certs) == 0
		}
		if empty {
			warning(result, doc, "sections", fmt.Sprintf("optional section %q has no selected content", section))
		}
	}
}

func validateSections(result *Result, doc *resolve.Document, sections []string) {
	valid := map[string]bool{
		"summary": true, "impact": true, "experience": true, "skills": true,
		"projects": true, "education": true, "certifications": true,
	}
	seen := make(map[string]bool)
	for i, section := range sections {
		field := fmt.Sprintf("sections[%d]", i)
		if !valid[section] {
			addError(result, doc, field, fmt.Sprintf("unknown section %q", section))
		}
		if seen[section] {
			addError(result, doc, field, fmt.Sprintf("duplicate section %q", section))
		}
		seen[section] = true
	}
}

func validateIDs[T any](result *Result, doc *resolve.Document, field string, items []T, id func(T) string) {
	seen := make(map[string]bool)
	for i, item := range items {
		itemID := id(item)
		itemField := fmt.Sprintf("%s[%d].id", field, i)
		require(result, doc, itemField, itemID)
		duplicateID(result, doc, itemField, itemID, seen)
	}
}

func duplicateID(result *Result, doc *resolve.Document, field, id string, seen map[string]bool) {
	if id == "" {
		return
	}
	if seen[id] {
		addError(result, doc, field, fmt.Sprintf("duplicate id %q", id))
	}
	seen[id] = true
}

func require(result *Result, doc *resolve.Document, field, value string) {
	if strings.TrimSpace(value) == "" {
		addError(result, doc, field, "value is required")
	}
}

func checkDate(result *Result, doc *resolve.Document, field, value string, optional bool) {
	if value == "" && optional {
		return
	}
	if value == "" {
		addError(result, doc, field, "date is required")
		return
	}
	if optional && (strings.EqualFold(value, "present") || strings.EqualFold(value, "current")) {
		return
	}
	for _, layout := range []string{"2006", "2006-01", "2006-01-02"} {
		if _, err := time.Parse(layout, value); err == nil {
			return
		}
	}
	addError(result, doc, field, fmt.Sprintf("invalid date %q; use YYYY, YYYY-MM, or YYYY-MM-DD", value))
}

func checkURL(result *Result, doc *resolve.Document, field, value string) {
	if value == "" {
		return
	}
	u, err := url.ParseRequestURI(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		addError(result, doc, field, fmt.Sprintf("invalid URL %q; expected an http or https URL", value))
	}
}

func checkRefs[T any](result *Result, doc *resolve.Document, field string, refs []T, getID func(T) string, exists func(T) bool) {
	for i, ref := range refs {
		if !exists(ref) {
			addError(result, doc, fmt.Sprintf("%s[%d].id", field, i), fmt.Sprintf("unknown reference %q", getID(ref)))
		}
	}
}

func addError(result *Result, doc *resolve.Document, field, message string) {
	result.Errors = append(result.Errors, Diagnostic{Path: doc.Path, Line: lineForField(doc.Raw, field), Field: field, Message: message})
}

func warning(result *Result, doc *resolve.Document, field, message string) {
	result.Warnings = append(result.Warnings, Diagnostic{Path: doc.Path, Line: lineForField(doc.Raw, field), Field: field, Message: message})
}

func lineForField(data []byte, field string) int {
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) == 0 {
		return 1
	}
	name := field
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		name = name[dot+1:]
	}
	if bracket := strings.IndexByte(name, '['); bracket >= 0 {
		name = name[:bracket]
	}
	var find func(*yaml.Node) int
	find = func(node *yaml.Node) int {
		if node.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(node.Content); i += 2 {
				key, value := node.Content[i], node.Content[i+1]
				if key.Value == name {
					return value.Line
				}
				if line := find(value); line != 0 {
					return line
				}
			}
		}
		for _, child := range node.Content {
			if line := find(child); line != 0 {
				return line
			}
		}
		return 0
	}
	if line := find(root.Content[0]); line != 0 {
		return line
	}
	return 1
}

func lineInError(err error) int {
	match := errorLine.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 1
	}
	var line int
	_, _ = fmt.Sscanf(match[1], "%d", &line)
	if line < 1 {
		return 1
	}
	return line
}

func fieldInError(err error) string {
	var envelopeField interface{ Field() string }
	if errorAs(err, &envelopeField) {
		return envelopeField.Field()
	}
	match := errorField.FindStringSubmatch(err.Error())
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func writeHuman(out io.Writer, path string, result Result) {
	if result.Valid {
		fmt.Fprintf(out, "Valid: %s\n", path)
	}
	for _, diagnostic := range result.Errors {
		writeDiagnostic(out, "error", diagnostic)
	}
	for _, diagnostic := range result.Warnings {
		writeDiagnostic(out, "warning", diagnostic)
	}
}

func writeDiagnostic(out io.Writer, severity string, diagnostic Diagnostic) {
	if diagnostic.Line > 0 {
		fmt.Fprintf(out, "%s:%d:", diagnostic.Path, diagnostic.Line)
	} else {
		fmt.Fprintf(out, "%s:", diagnostic.Path)
	}
	if diagnostic.Field != "" {
		fmt.Fprintf(out, " %s:", diagnostic.Field)
	}
	fmt.Fprintf(out, " %s: %s\n", severity, diagnostic.Message)
}

func errorAs(err error, target any) bool {
	return errors.As(err, target)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func findSkillGroup(groups []schema.SkillGroup, id string) (schema.SkillGroup, bool) {
	for _, group := range groups {
		if group.ID == id {
			return group, true
		}
	}
	return schema.SkillGroup{}, false
}

func findEmployer(employers []schema.Employer, idOrName string) (schema.Employer, bool) {
	for _, employer := range employers {
		if employer.ID == idOrName || employer.Name == idOrName {
			return employer, true
		}
	}
	return schema.Employer{}, false
}

func hasImpact(profile *schema.Profile, id string) bool {
	for _, item := range profile.Impact {
		if item.ID == id {
			return true
		}
	}
	return false
}

func hasProject(profile *schema.Profile, id string) bool {
	for _, item := range profile.Projects {
		if item.ID == id {
			return true
		}
	}
	return false
}

func hasEducation(profile *schema.Profile, id string) bool {
	for _, item := range profile.Education {
		if item.ID == id {
			return true
		}
	}
	return false
}

func hasCert(profile *schema.Profile, id string) bool {
	for _, item := range profile.Certs {
		if item.ID == id {
			return true
		}
	}
	return false
}
