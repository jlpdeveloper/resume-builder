package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"github.com/jlpdeveloper/resume-builder/internal/validate"
	"gopkg.in/yaml.v3"
)

// Source identifies where content originated and where it was selected.
type Source struct {
	Kind       string `json:"kind"`
	File       string `json:"file"`
	SelectedBy string `json:"selected_by,omitempty"`
	Inherited  bool   `json:"inherited"`
}

// SourceFile identifies a file in the dependency graph.
type SourceFile struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

// Role is an experience role with its content source.
type Role struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Start           string   `json:"start"`
	End             string   `json:"end,omitempty"`
	Accomplishments []string `json:"accomplishments,omitempty"`
	Source          Source   `json:"source"`
}

// Header contains the résumé's contact details.
type Header struct {
	Name        string   `json:"name"`
	Phone       string   `json:"phone,omitempty"`
	Email       string   `json:"email,omitempty"`
	Location    string   `json:"location,omitempty"`
	GitHub      string   `json:"github,omitempty"`
	LinkedIn    string   `json:"linkedin,omitempty"`
	Credentials []string `json:"credentials,omitempty"`
	Portfolio   []Link   `json:"portfolio,omitempty"`
}

// Link is a labeled header URL.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Target identifies the job this résumé targets.
type Target struct {
	Company string `json:"company"`
	Role    string `json:"role"`
	URL     string `json:"url,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// Metadata contains optional PDF document properties.
type Metadata struct {
	Title   string `json:"title,omitempty"`
	Author  string `json:"author,omitempty"`
	Subject string `json:"subject,omitempty"`
}

// Item is one resolved entry in an inspect section.
type Item struct {
	Kind    string         `json:"kind"`
	ID      string         `json:"id,omitempty"`
	Name    string         `json:"name,omitempty"`
	Title   string         `json:"title,omitempty"`
	Text    string         `json:"text,omitempty"`
	Details map[string]any `json:"details,omitempty"`
	Roles   []Role         `json:"roles,omitempty"`
	Source  Source         `json:"source"`
}

// Section preserves the resolved display order and heading for each section.
type Section struct {
	Name    string `json:"name"`
	Heading string `json:"heading"`
	Items   []Item `json:"items"`
}

// Report is the resolved résumé with source attribution for each entry.
type Report struct {
	Valid          bool         `json:"valid"`
	ResumePath     string       `json:"resume_path"`
	Header         Header       `json:"header"`
	HeaderSource   Source       `json:"header_source"`
	Target         Target       `json:"target"`
	TargetSource   Source       `json:"target_source"`
	Headline       string       `json:"headline,omitempty"`
	HeadlineSource Source       `json:"headline_source"`
	PageLimit      int          `json:"page_limit,omitempty"`
	Metadata       Metadata     `json:"metadata"`
	Sources        []SourceFile `json:"sources"`
	Sections       []Section    `json:"sections"`
}

type validationFailure struct {
	result validate.Result
}

func (e *validationFailure) Error() string {
	return "resume validation failed"
}

// Inspect resolves a resume and builds its content report.
func Inspect(path string) (*Report, error) {
	validationResult := validate.Validate(path)
	if !validationResult.Valid {
		return nil, &validationFailure{result: validationResult}
	}

	graph, err := (&resolve.Resolver{}).Resolve(path)
	if err != nil {
		return nil, err
	}
	resolved, err := resolve.ResolveContent(graph)
	if err != nil {
		return nil, err
	}

	var base schema.Base
	var resume schema.Resume
	var hasBase bool
	for _, doc := range graph.Documents {
		switch doc.Kind {
		case schema.KindBase:
			hasBase = true
			if err := yaml.Unmarshal(doc.Raw, &base); err != nil {
				return nil, err
			}
		case schema.KindResume:
			if err := yaml.Unmarshal(doc.Raw, &resume); err != nil {
				return nil, err
			}
		}
	}

	report := &Report{
		Valid:          true,
		ResumePath:     resolved.ResumePath,
		Header:         toHeader(resolved.Header),
		HeaderSource:   makeSource("profile", resolved.ProfilePath, "", "resume"),
		Target:         toTarget(resolved.Target),
		TargetSource:   makeSource("resume", resolved.ResumePath, "resume", "resume"),
		Headline:       resolved.Headline,
		HeadlineSource: makeSource("resume", resolved.ResumePath, "resume", "resume"),
		PageLimit:      resolved.PageLimit,
		Metadata:       toMetadata(resolved.Metadata),
		Sources:        make([]SourceFile, 0, len(graph.Documents)),
		Sections:       make([]Section, 0, len(resolved.Sections)),
	}
	for _, doc := range graph.Documents {
		report.Sources = append(report.Sources, SourceFile{Kind: string(doc.Kind), Path: doc.Path})
	}
	sort.Slice(report.Sources, func(i, j int) bool {
		if report.Sources[i].Kind == report.Sources[j].Kind {
			return report.Sources[i].Path < report.Sources[j].Path
		}
		return report.Sources[i].Kind < report.Sources[j].Kind
	})

	for _, name := range resolved.Sections {
		section := Section{Name: name, Heading: heading(name), Items: []Item{}}
		switch name {
		case "summary":
			if resolved.Summary != nil {
				section.Items = append(section.Items, Item{
					Kind:   "summary",
					ID:     resolved.Summary.ID,
					Text:   resolved.Summary.Text,
					Source: sourceForResolved(resolved, resolved.Summary.Source, "resume"),
				})
			}
		case "impact":
			for _, impact := range resolved.Impact {
				section.Items = append(section.Items, Item{
					Kind:   "impact",
					ID:     impact.ID,
					Text:   impact.Text,
					Source: sourceForResolved(resolved, impact.Source, "resume"),
				})
			}
		case "skills":
			for _, group := range resolved.Skills {
				origin := group.Source
				selectedBy := group.Source
				if origin == "resume" {
					origin = "profile"
				}
				if group.Source == "resume" {
					selectedBy = "resume"
				} else if len(resume.Skills) > 0 {
					selectedBy = "resume"
				}
				section.Items = append(section.Items, Item{
					Kind:    "skills",
					ID:      group.ID,
					Name:    group.Name,
					Details: map[string]any{"skills": group.Skills},
					Source:  sourceForResolved(resolved, origin, selectedBy),
				})
			}
		case "experience":
			selectedBy := selectedBy(len(resume.Experience), hasBase && len(base.Employment) > 0)
			for _, employer := range resolved.Experience {
				employerSource := sourceForResolved(resolved, employer.Source, selectedBy)
				item := Item{
					Kind: "experience",
					ID:   employer.ID,
					Name: employer.Name,
					Details: map[string]any{
						"location": employer.Location,
						"link":     employer.Link,
					},
					Source: employerSource,
					Roles:  make([]Role, 0, len(employer.Roles)),
				}
				for _, role := range employer.Roles {
					item.Roles = append(item.Roles, Role{
						ID:              role.ID,
						Title:           role.Title,
						Start:           role.Start,
						End:             role.End,
						Accomplishments: role.Accomplishments,
						Source:          sourceForResolved(resolved, role.Source, selectedBy),
					})
				}
				section.Items = append(section.Items, item)
			}
		case "projects":
			by := selectedBy(len(resume.Projects), hasBase && len(base.Projects) > 0)
			for _, project := range resolved.Projects {
				section.Items = append(section.Items, Item{
					Kind:  "project",
					ID:    project.ID,
					Name:  project.Name,
					Title: project.Role,
					Details: map[string]any{
						"start": project.Start, "end": project.End, "link": project.Link,
						"tech": project.Tech, "accomplishments": project.Accomplishments,
					},
					Source: makeSource("profile", resolved.ProfilePath, by, "resume"),
				})
			}
		case "education":
			by := selectedBy(len(resume.Education), hasBase && len(base.Education) > 0)
			for _, education := range resolved.Education {
				section.Items = append(section.Items, Item{
					Kind:  "education",
					ID:    education.ID,
					Name:  education.Institution,
					Title: education.Degree,
					Details: map[string]any{
						"field": education.Field, "graduation": education.Graduation, "honors": education.Honors,
					},
					Source: makeSource("profile", resolved.ProfilePath, by, "resume"),
				})
			}
		case "certifications":
			by := selectedBy(len(resume.Certs), hasBase && len(base.Certs) > 0)
			for _, cert := range resolved.Certs {
				section.Items = append(section.Items, Item{
					Kind:  "certification",
					ID:    cert.ID,
					Name:  cert.Name,
					Title: cert.Issuer,
					Details: map[string]any{
						"date_earned": cert.DateEarned, "expiration": cert.Expiration,
						"credential_url": cert.CredentialURL,
					},
					Source: makeSource("profile", resolved.ProfilePath, by, "resume"),
				})
			}
		}
		report.Sections = append(report.Sections, section)
	}
	return report, nil
}

// Run implements the inspect command.
func Run(args []string, stdout, stderr io.Writer, jsonOutput bool) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(stderr, "Usage: resume inspect PATH")
		return 2
	}
	report, err := Inspect(args[0])
	if err != nil {
		var failure *validationFailure
		if errors.As(err, &failure) {
			if jsonOutput {
				if encodeErr := json.NewEncoder(stdout).Encode(failure.result); encodeErr != nil {
					fmt.Fprintf(stderr, "write inspect output: %v\n", encodeErr)
					return 1
				}
			} else {
				writeDiagnostics(stderr, failure.result)
			}
			return 1
		}
		if jsonOutput {
			_ = json.NewEncoder(stdout).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(report); err != nil {
			fmt.Fprintf(stderr, "write inspect output: %v\n", err)
			return 1
		}
	} else {
		writeReport(stdout, report)
	}
	return 0
}

func sourceForResolved(resolved *resolve.Resolved, kind, selectedBy string) Source {
	return makeSource(kind, sourcePath(resolved, kind), selectedBy, "resume")
}

func toHeader(header schema.Header) Header {
	links := make([]Link, 0, len(header.Portfolio))
	for _, link := range header.Portfolio {
		links = append(links, Link{Label: link.Label, URL: link.URL})
	}
	return Header{
		Name: header.Name, Phone: header.Phone, Email: header.Email, Location: header.Location,
		GitHub: header.GitHub, LinkedIn: header.LinkedIn,
		Credentials: header.Credentials, Portfolio: links,
	}
}

func toTarget(target schema.Target) Target {
	return Target{Company: target.Company, Role: target.Role, URL: target.URL, Notes: target.Notes}
}

func toMetadata(metadata schema.PDFMeta) Metadata {
	return Metadata{Title: metadata.Title, Author: metadata.Author, Subject: metadata.Subject}
}

func sourcePath(resolved *resolve.Resolved, kind string) string {
	switch kind {
	case "profile":
		return resolved.ProfilePath
	case "base":
		return resolved.BasePath
	case "resume":
		return resolved.ResumePath
	default:
		return ""
	}
}

func makeSource(kind, file, selectedBy, target string) Source {
	return Source{Kind: kind, File: file, SelectedBy: selectedBy, Inherited: kind != target}
}

func selectedBy(resumeCount int, baseSelected bool) string {
	if resumeCount > 0 {
		return "resume"
	}
	if baseSelected {
		return "base"
	}
	return "resume"
}

func heading(name string) string {
	if name == "certifications" {
		return "Certifications"
	}
	if name == "impact" {
		return "Impact"
	}
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func writeDiagnostics(out io.Writer, result validate.Result) {
	for _, diagnostic := range result.Errors {
		fmt.Fprintf(out, "%s:%d: %s: error: %s\n", diagnostic.Path, diagnostic.Line, diagnostic.Field, diagnostic.Message)
	}
	for _, diagnostic := range result.Warnings {
		fmt.Fprintf(out, "%s:%d: %s: warning: %s\n", diagnostic.Path, diagnostic.Line, diagnostic.Field, diagnostic.Message)
	}
}

func writeReport(out io.Writer, report *Report) {
	fmt.Fprintf(out, "%s — %s\n", report.Target.Role, report.Target.Company)
	fmt.Fprintf(out, "Name: %s\n", report.Header.Name)
	for _, detail := range []struct{ label, value string }{
		{"Phone", report.Header.Phone},
		{"Email", report.Header.Email},
		{"Location", report.Header.Location},
		{"GitHub", report.Header.GitHub},
		{"LinkedIn", report.Header.LinkedIn},
	} {
		if detail.value != "" {
			fmt.Fprintf(out, "%s: %s\n", detail.label, detail.value)
		}
	}
	if len(report.Header.Credentials) > 0 {
		fmt.Fprintf(out, "Credentials: %s\n", strings.Join(report.Header.Credentials, ", "))
	}
	for _, link := range report.Header.Portfolio {
		fmt.Fprintf(out, "Portfolio: %s (%s)\n", link.Label, link.URL)
	}
	fmt.Fprintf(out, "Header source: %s (%s)\n", report.HeaderSource.Kind, report.HeaderSource.File)
	fmt.Fprintf(out, "Target source: %s (%s)\n", report.TargetSource.Kind, report.TargetSource.File)
	if report.Target.URL != "" {
		fmt.Fprintf(out, "Target URL: %s\n", report.Target.URL)
	}
	if report.Target.Notes != "" {
		fmt.Fprintf(out, "Target notes: %s\n", report.Target.Notes)
	}
	if report.Headline != "" {
		fmt.Fprintf(out, "Headline: %s\n", report.Headline)
		fmt.Fprintf(out, "Headline source: %s (%s)\n", report.HeadlineSource.Kind, report.HeadlineSource.File)
	}
	if report.PageLimit > 0 {
		fmt.Fprintf(out, "Page limit: %d\n", report.PageLimit)
	}
	for _, detail := range []struct{ label, value string }{
		{"PDF title", report.Metadata.Title},
		{"PDF author", report.Metadata.Author},
		{"PDF subject", report.Metadata.Subject},
	} {
		if detail.value != "" {
			fmt.Fprintf(out, "%s: %s\n", detail.label, detail.value)
		}
	}
	fmt.Fprintln(out, "")
	for index, section := range report.Sections {
		fmt.Fprintf(out, "%d. %s\n", index+1, section.Heading)
		if len(section.Items) == 0 {
			fmt.Fprintln(out, "  (no content)")
			continue
		}
		for _, item := range section.Items {
			writeItem(out, item)
		}
	}
}

func writeItem(out io.Writer, item Item) {
	label := item.Name
	if label == "" {
		label = item.Title
	}
	if label != "" {
		fmt.Fprintf(out, "  - %s", label)
	} else {
		fmt.Fprint(out, "  -")
	}
	if item.Title != "" && item.Name != "" {
		fmt.Fprintf(out, " — %s", item.Title)
	}
	if item.Text != "" {
		fmt.Fprintf(out, ": %s", item.Text)
	}
	fmt.Fprintf(out, " [%s: %s", item.Source.Kind, item.Source.File)
	if item.Source.Inherited {
		fmt.Fprint(out, "; inherited")
	} else {
		fmt.Fprint(out, "; target-specific")
	}
	if item.Source.SelectedBy != "" && item.Source.SelectedBy != item.Source.Kind {
		fmt.Fprintf(out, "; selected by %s", item.Source.SelectedBy)
	}
	fmt.Fprintln(out, "]")
	if skills, ok := item.Details["skills"].([]string); ok {
		fmt.Fprintf(out, "    %s\n", strings.Join(skills, ", "))
	}
	for _, role := range item.Roles {
		dateRange := role.Start
		if role.End != "" {
			dateRange += "–" + role.End
		}
		fmt.Fprintf(out, "    %s (%s) [%s: %s]\n", role.Title, dateRange, role.Source.Kind, role.Source.File)
		for _, bullet := range role.Accomplishments {
			fmt.Fprintf(out, "      • %s\n", bullet)
		}
	}
	for _, key := range []string{
		"institution", "field", "graduation", "honors", "issuer", "tech", "location", "start", "end",
		"date_earned", "expiration", "credential_url", "link",
	} {
		if value, ok := item.Details[key].(string); ok && value != "" {
			fmt.Fprintf(out, "    %s: %s\n", key, value)
		}
	}
	if accomplishments, ok := item.Details["accomplishments"].([]string); ok {
		for _, bullet := range accomplishments {
			fmt.Fprintf(out, "    • %s\n", bullet)
		}
	}
}
