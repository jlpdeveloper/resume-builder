package build

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	resumeTemplate "github.com/jlpdeveloper/resume-builder/internal/template"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestRender_EmbeddedFontsMetadataLinksAndPagination(t *testing.T) {
	resolved := &resolve.Resolved{
		Header: schema.Header{
			Name: "Ada Lovelace", Email: "ada@example.com", GitHub: "https://github.com/ada",
			Portfolio: []schema.Link{{Label: "Portfolio", URL: "https://example.com/ada"}},
		},
		Target:   schema.Target{Company: "Analytical Engines", Role: "Engineer", URL: "https://example.com/jobs"},
		Headline: "Builds dependable systems",
		Summary:  &resolve.ResolvedSummary{ID: "summary-1", Text: "Experienced systems engineer."},
		Metadata: schema.PDFMeta{Title: "Ada Lovelace Resume", Author: "Ada Lovelace", Subject: "Engineering"},
		Sections: []string{"summary", "impact", "skills", "experience", "projects", "education", "certifications"},
		Impact:   make([]resolve.ResolvedImpact, 80),
		Skills: []resolve.ResolvedSkillGroup{{
			ID: "technical", Name: "Technical", Skills: []string{"Go", "Distributed Systems"}, Source: "profile",
		}},
		Experience: []resolve.ResolvedEmployer{{
			ID: "acme", Name: "Acme", Location: "Remote", Link: "https://example.com/acme",
			Roles:  []resolve.ResolvedRole{{ID: "engineer", Title: "Engineer", Start: "2020", Accomplishments: []string{"Designed resilient systems."}, Source: "profile"}},
			Source: "profile",
		}},
		Projects:  []schema.Project{{ID: "project-1", Name: "Open Project", Role: "Maintainer", Link: "https://example.com/project", Accomplishments: []string{"Built useful software."}}},
		Education: []schema.Education{{ID: "education-1", Institution: "Example University", Degree: "BS", Graduation: "2018"}},
		Certs:     []schema.Certification{{ID: "cert-1", Name: "Go Certified", Issuer: "Example Org", CredentialURL: "https://example.com/cert"}},
	}
	for i := range resolved.Impact {
		resolved.Impact[i] = resolve.ResolvedImpact{ID: "impact-" + strings.Repeat("x", i+1), Text: "Improved reliability across production services."}
	}

	result, err := Render(resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(result.PDF, []byte("%PDF-")) {
		t.Fatalf("expected a PDF document, got %q", result.PDF[:min(20, len(result.PDF))])
	}
	if result.PageCount < 2 {
		t.Errorf("expected long content to paginate, got %d page(s)", result.PageCount)
	}
	for _, marker := range [][]byte{
		[]byte("/FontFile2"), []byte("/ToUnicode"),
		[]byte("https://github.com/ada"),
	} {
		if !bytes.Contains(result.PDF, marker) {
			t.Errorf("generated PDF is missing %q", marker)
		}
	}
	info, err := api.PDFInfo(context.Background(), bytes.NewReader(result.PDF), "resume.pdf", nil, false, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != resolved.Metadata.Title || info.Author != resolved.Metadata.Author || info.Subject != resolved.Metadata.Subject {
		t.Errorf("PDF metadata mismatch: %+v", info)
	}
	pdfPath := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(pdfPath, result.PDF, 0600); err != nil {
		t.Fatal(err)
	}
	pdfFile, reader, err := pdf.Open(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	defer pdfFile.Close()
	textReader, err := reader.GetPlainText()
	if err != nil {
		t.Fatal(err)
	}
	visibleText, err := io.ReadAll(textReader)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Ada Lovelace", "Experienced systems engineer.", "Designed resilient systems."} {
		if !strings.Contains(string(visibleText), want) {
			t.Errorf("extracted PDF text does not contain %q: %s", want, visibleText)
		}
	}
}

func TestRender_RejectsNilResume(t *testing.T) {
	if _, err := Render(nil, nil); err == nil {
		t.Fatal("expected nil resolved resume to return an error")
	}
}

func TestStandardTemplateMapsToRendererSettings(t *testing.T) {
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	r := newRenderer(standard)
	if r.regular != "ResumeSans" || r.bold != "ResumeSans" || r.nameSize != 21 || r.contactSize != 8 {
		t.Errorf("font settings were not mapped: %+v", r)
	}
	if r.headingAlign != align.Center || !r.contactCompact {
		t.Errorf("header settings were not mapped: %+v", r)
	}
	if r.headingColor.Red != 31 || r.headingColor.Green != 78 || r.headingColor.Blue != 121 {
		t.Errorf("heading color was not mapped: %+v", r.headingColor)
	}
	if r.ruleColor.Red != 142 || r.ruleColor.Green != 170 || r.ruleColor.Blue != 219 {
		t.Errorf("rule color was not mapped: %+v", r.ruleColor)
	}
	if !boolValue(r.template.Headings.Rule, false) || !footerNameEnabled(r.template.Footer.Name) || !footerPageEnabled(r.template.Footer.Page) {
		t.Errorf("heading/footer settings were not mapped: %+v", r.template)
	}
}
