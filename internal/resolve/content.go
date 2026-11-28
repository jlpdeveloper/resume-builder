package resolve

import (
	"fmt"
	"slices"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"gopkg.in/yaml.v3"
)

// Resolved is the fully resolved résumé ready for rendering.
// Every item identifies its source file so the caller can distinguish
// inherited content from target-specific content.
type Resolved struct {
	// Source files
	ProfilePath  string
	BasePath     string
	ResumePath   string
	TemplatePath string

	// Header from profile
	Header schema.Header

	// Target from resume
	Target schema.Target

	// Headline is target-specific
	Headline string

	// Summary — only one may appear
	Summary *ResolvedSummary

	// Impact statements in order
	Impact []ResolvedImpact

	// Skills in order
	Skills []ResolvedSkillGroup

	// Experience in order
	Experience []ResolvedEmployer

	// Projects in order
	Projects []schema.Project

	// Education in order
	Education []schema.Education

	// Certifications in order
	Certs []schema.Certification

	// Section order (controls rendering order)
	Sections []string

	// Page limit
	PageLimit int

	// PDF metadata
	Metadata schema.PDFMeta

	// Template (presentation only, no content)
	Template *schema.Template
}

// ResolvedSummary tracks the source of a summary.
type ResolvedSummary struct {
	ID     string
	Text   string
	Source string // "profile" or "resume"
}

// ResolvedImpact tracks the source of an impact statement.
type ResolvedImpact struct {
	ID     string
	Text   string
	Source string // "profile" or "resume"
}

// ResolvedSkillGroup is a skill group with source tracking.
type ResolvedSkillGroup struct {
	ID     string
	Name   string
	Skills []string
	Source string // "profile", "base", or "resume"
}

// ResolvedEmployer is an employer with selected roles and source tracking.
type ResolvedEmployer struct {
	ID       string
	Name     string
	Location string
	Link     string
	Roles    []ResolvedRole
	Source   string // "profile" or "base"
}

// ResolvedRole is a role with selected accomplishments.
type ResolvedRole struct {
	ID              string
	Title           string
	Start           string
	End             string
	Scope           string
	Accomplishments []string
	Source          string // "profile" or "base"
}

// ResolveContent takes a resolved dependency graph and produces a fully
// resolved résumé by merging base defaults with resume overrides.
func ResolveContent(graph *Graph) (*Resolved, error) {
	if graph == nil || len(graph.Documents) == 0 {
		return nil, fmt.Errorf("empty graph")
	}

	// Find documents by kind.
	var resumeDoc, baseDoc, profileDoc, templateDoc *Document
	for _, doc := range graph.Documents {
		switch doc.Kind {
		case schema.KindResume:
			resumeDoc = doc
		case schema.KindBase:
			baseDoc = doc
		case schema.KindProfile:
			profileDoc = doc
		case schema.KindTemplate:
			templateDoc = doc
		}
	}

	if resumeDoc == nil {
		return nil, fmt.Errorf("graph must contain a resume document")
	}
	if profileDoc == nil {
		return nil, fmt.Errorf("graph must contain a profile document")
	}

	// Parse the full structs.
	profile, err := parseProfile(profileDoc)
	if err != nil {
		return nil, err
	}

	base := &schema.Base{}
	if baseDoc != nil {
		base, err = parseBase(baseDoc)
		if err != nil {
			return nil, err
		}
	}

	resume, err := parseResume(resumeDoc)
	if err != nil {
		return nil, err
	}

	template := &schema.Template{}
	if templateDoc != nil {
		template, err = parseTemplate(templateDoc)
		if err != nil {
			return nil, err
		}
	}

	r := &Resolved{
		ProfilePath: profileDoc.Path,
		ResumePath:  resumeDoc.Path,
		Header:      profile.Header,
		Target:      resume.Target,
		Headline:    resume.Headline,
	}

	if baseDoc != nil {
		r.BasePath = baseDoc.Path
	}
	if templateDoc != nil {
		r.TemplatePath = templateDoc.Path
		r.Template = template
	}

	// Resolve summary.
	r.Summary, err = resolveSummary(profile, resume)
	if err != nil {
		return nil, err
	}

	// Resolve impact statements.
	r.Impact = resolveImpact(profile, resume)

	// Resolve skills.
	r.Skills = resolveSkills(profile, base, resume)

	// Resolve experience.
	r.Experience = resolveExperience(profile, base, resume)

	// Resolve projects.
	r.Projects = resolveProjects(profile, base, resume)

	// Resolve education.
	r.Education = resolveEducation(profile, base, resume)

	// Resolve certifications.
	r.Certs = resolveCerts(profile, base, resume)

	// Resolve section order.
	r.Sections = resolveSections(base, resume)

	// Resolve page limit (resume overrides base and template defaults).
	r.PageLimit = resolvePageLimit(base, resume, template)

	// Resolve metadata.
	r.Metadata = resume.Metadata

	return r, nil
}

// resolveSummary returns the single resolved summary.
// Resume can provide an inline summary or reference a profile summary.
// If the resume doesn't specify a summary, there is no summary.
func resolveSummary(profile *schema.Profile, resume *schema.Resume) (*ResolvedSummary, error) {
	if resume.Summary == nil {
		return nil, nil
	}

	if resume.Summary.Text != "" {
		return &ResolvedSummary{
			Text:   resume.Summary.Text,
			Source: "resume",
		}, nil
	}

	if resume.Summary.ID != "" {
		for _, s := range profile.Summaries {
			if s.ID == resume.Summary.ID {
				return &ResolvedSummary{
					ID:     s.ID,
					Text:   s.Text,
					Source: "profile",
				}, nil
			}
		}
		return nil, fmt.Errorf("summary id %q not found in profile", resume.Summary.ID)
	}

	return nil, nil
}

// resolveImpact merges impact statements from the resume.
// Each can be a profile reference or inline text.
func resolveImpact(profile *schema.Profile, resume *schema.Resume) []ResolvedImpact {
	var result []ResolvedImpact

	for _, sel := range resume.Impact {
		if sel.Text != "" {
			result = append(result, ResolvedImpact{
				ID:     sel.ID,
				Text:   sel.Text,
				Source: "resume",
			})
		} else if sel.ID != "" {
			for _, s := range profile.Impact {
				if s.ID == sel.ID {
					result = append(result, ResolvedImpact{
						ID:     s.ID,
						Text:   s.Text,
						Source: "profile",
					})
					break
				}
			}
		}
	}

	return result
}

// resolveSkills merges skill groups from base and resume.
// Base provides default skill selections; resume can override.
func resolveSkills(profile *schema.Profile, base *schema.Base, resume *schema.Resume) []ResolvedSkillGroup {
	// Build a map of profile skill groups by ID.
	profileSkills := make(map[string]schema.SkillGroup)
	for _, sg := range profile.Skills {
		profileSkills[sg.ID] = sg
	}

	// Start with base skill selections if they exist.
	var skillSels []schema.SkillSel
	var source string
	if base != nil && len(resume.Skills) == 0 {
		// Base skills are inline skill groups, not selections.
		// Convert base skill groups to resolved skill groups.
		var result []ResolvedSkillGroup
		for _, sg := range base.Skills {
			result = append(result, ResolvedSkillGroup{
				ID:     sg.ID,
				Name:   sg.Name,
				Skills: sg.Skills,
				Source: "base",
			})
		}
		return result
	} else if len(resume.Skills) > 0 {
		skillSels = resume.Skills
		source = "resume"
	} else {
		return nil
	}

	// Resolve resume skill selections against profile.
	var result []ResolvedSkillGroup
	for _, sel := range skillSels {
		if sg, ok := profileSkills[sel.Group]; ok {
			skills := sg.Skills
			if len(sel.Skills) > 0 {
				// Filter to only selected skills, in the order specified.
				skills = nil
				for _, s := range sel.Skills {
					if slices.Contains(sg.Skills, s) {
						skills = append(skills, s)
					}
				}
			}
			result = append(result, ResolvedSkillGroup{
				ID:     sg.ID,
				Name:   sg.Name,
				Skills: skills,
				Source: source,
			})
		}
	}

	return result
}

// resolveExperience merges experience from base and resume.
// Base can define employer-level experience; resume selects companies and roles.
func resolveExperience(profile *schema.Profile, base *schema.Base, resume *schema.Resume) []ResolvedEmployer {
	// Build maps of profile employers by ID.
	profileEmployers := make(map[string]schema.Employer)
	for _, e := range profile.Employment {
		profileEmployers[e.ID] = e
	}

	// If resume has no experience selections, use base employment if present.
	if len(resume.Experience) == 0 {
		if base != nil && len(base.Employment) > 0 {
			var result []ResolvedEmployer
			for _, e := range base.Employment {
				roles := makeResolvedRoles(e.Roles, "base")
				result = append(result, ResolvedEmployer{
					ID:       e.ID,
					Name:     e.Name,
					Location: e.Location,
					Link:     e.Link,
					Roles:    roles,
					Source:   "base",
				})
			}
			return result
		}
		return nil
	}

	// Resume has explicit experience selections.
	// Build a set of selected company IDs to determine ordering.
	var result []ResolvedEmployer
	seen := make(map[string]bool)

	for _, sel := range resume.Experience {
		// Find the employer in profile.
		emp, ok := profileEmployers[sel.Company]
		if !ok {
			// Try to find by name.
			for _, e := range profile.Employment {
				if e.Name == sel.Company {
					emp = e
					ok = true
					break
				}
			}
		}
		if !ok {
			continue
		}
		if seen[emp.ID] {
			continue
		}
		seen[emp.ID] = true

		// Select roles.
		var roles []ResolvedRole
		if len(sel.Roles) == 0 {
			// No role filter: include all roles from profile.
			roles = makeResolvedRoles(emp.Roles, "profile")
		} else {
			// Select specific roles in the order specified by the resume.
			roleMap := make(map[string]schema.Role)
			for _, r := range emp.Roles {
				roleMap[r.ID] = r
			}
			for _, roleID := range sel.Roles {
				r, ok := roleMap[roleID]
				if !ok {
					continue
				}
				accs := r.Accomplishments
				if len(sel.Bullets) > 0 {
					// Filter to only selected bullets.
					accs = nil
					for _, b := range sel.Bullets {
						if slices.Contains(r.Accomplishments, b) {
							accs = append(accs, b)
						}
					}
				}
				roles = append(roles, ResolvedRole{
					ID:              r.ID,
					Title:           r.Title,
					Start:           r.Start,
					End:             r.End,
					Scope:           r.Scope,
					Accomplishments: accs,
					Source:          "profile",
				})
			}
		}

		result = append(result, ResolvedEmployer{
			ID:       emp.ID,
			Name:     emp.Name,
			Location: emp.Location,
			Link:     emp.Link,
			Roles:    roles,
			Source:   "profile",
		})
	}

	return result
}

// resolveProjects resolves project references from base and resume.
func resolveProjects(profile *schema.Profile, base *schema.Base, resume *schema.Resume) []schema.Project {
	// Build map of profile projects by ID.
	profileProjects := make(map[string]schema.Project)
	for _, p := range profile.Projects {
		profileProjects[p.ID] = p
	}

	// If resume has no project selections, use base projects if present.
	var refs []schema.Ref
	if len(resume.Projects) > 0 {
		refs = resume.Projects
	} else if base != nil && len(base.Projects) > 0 {
		refs = make([]schema.Ref, 0, len(base.Projects))
		for _, br := range base.Projects {
			refs = append(refs, schema.Ref{ID: br.ID})
		}
	} else {
		return nil
	}

	var result []schema.Project
	for _, ref := range refs {
		if p, ok := profileProjects[ref.ID]; ok {
			result = append(result, p)
		}
	}

	return result
}

// resolveEducation resolves education references from base and resume.
func resolveEducation(profile *schema.Profile, base *schema.Base, resume *schema.Resume) []schema.Education {
	// Build map of profile education by ID.
	profileEdu := make(map[string]schema.Education)
	for _, e := range profile.Education {
		profileEdu[e.ID] = e
	}

	// If resume has no education selections, use base education if present.
	var refs []schema.Ref
	if len(resume.Education) > 0 {
		refs = resume.Education
	} else if base != nil && len(base.Education) > 0 {
		refs = make([]schema.Ref, 0, len(base.Education))
		for _, br := range base.Education {
			refs = append(refs, schema.Ref{ID: br.ID})
		}
	} else {
		return nil
	}

	var result []schema.Education
	for _, ref := range refs {
		if e, ok := profileEdu[ref.ID]; ok {
			result = append(result, e)
		}
	}

	return result
}

// resolveCerts resolves certification references from base and resume.
func resolveCerts(profile *schema.Profile, base *schema.Base, resume *schema.Resume) []schema.Certification {
	// Build map of profile certs by ID.
	profileCerts := make(map[string]schema.Certification)
	for _, c := range profile.Certs {
		profileCerts[c.ID] = c
	}

	// If resume has no cert selections, use base certs if present.
	var refs []schema.Ref
	if len(resume.Certs) > 0 {
		refs = resume.Certs
	} else if base != nil && len(base.Certs) > 0 {
		refs = make([]schema.Ref, 0, len(base.Certs))
		for _, br := range base.Certs {
			refs = append(refs, schema.Ref{ID: br.ID})
		}
	} else {
		return nil
	}

	var result []schema.Certification
	for _, ref := range refs {
		if c, ok := profileCerts[ref.ID]; ok {
			result = append(result, c)
		}
	}

	return result
}

// resolveSections determines the section order.
// Resume sections override base sections.
func resolveSections(base *schema.Base, resume *schema.Resume) []string {
	if len(resume.Sections) > 0 {
		return resume.Sections
	}
	if base != nil && len(base.Sections) > 0 {
		return base.Sections
	}
	// Default section order.
	return []string{
		"summary",
		"impact",
		"experience",
		"skills",
		"projects",
		"education",
		"certifications",
	}
}

// resolvePageLimit prefers the resume, then base, then template limit.
func resolvePageLimit(base *schema.Base, resume *schema.Resume, template *schema.Template) int {
	if resume.PageLimit > 0 {
		return resume.PageLimit
	}
	if base != nil && base.PageLimit > 0 {
		return base.PageLimit
	}
	if template != nil && template.PageBreaks.PageLimit > 0 {
		return template.PageBreaks.PageLimit
	}
	return 0
}

// makeResolvedRoles converts profile roles to resolved roles.
func makeResolvedRoles(roles []schema.Role, source string) []ResolvedRole {
	result := make([]ResolvedRole, 0, len(roles))
	for _, r := range roles {
		result = append(result, ResolvedRole{
			ID:              r.ID,
			Title:           r.Title,
			Start:           r.Start,
			End:             r.End,
			Scope:           r.Scope,
			Accomplishments: r.Accomplishments,
			Source:          source,
		})
	}
	return result
}

// ---- Parsing helpers ----

func parseProfile(doc *Document) (*schema.Profile, error) {
	var profile schema.Profile
	if err := yaml.Unmarshal(doc.Raw, &profile); err != nil {
		return nil, &ParseError{path: doc.Path, err: err}
	}
	profile.Envelope = doc.Envelope
	return &profile, nil
}

func parseBase(doc *Document) (*schema.Base, error) {
	var base schema.Base
	if err := yaml.Unmarshal(doc.Raw, &base); err != nil {
		return nil, &ParseError{path: doc.Path, err: err}
	}
	base.Envelope = doc.Envelope
	return &base, nil
}

func parseResume(doc *Document) (*schema.Resume, error) {
	var resume schema.Resume
	if err := yaml.Unmarshal(doc.Raw, &resume); err != nil {
		return nil, &ParseError{path: doc.Path, err: err}
	}
	resume.Envelope = doc.Envelope
	return &resume, nil
}

func parseTemplate(doc *Document) (*schema.Template, error) {
	var template schema.Template
	if err := yaml.Unmarshal(doc.Raw, &template); err != nil {
		return nil, &ParseError{path: doc.Path, err: err}
	}
	template.Envelope = doc.Envelope
	return &template, nil
}
