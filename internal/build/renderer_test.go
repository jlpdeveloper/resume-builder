package build

import (
	"bytes"
	"context"
	"fmt"
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
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	showPageNumbers := false
	standard.Footer.Page = &showPageNumbers
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

	result, err := Render(resolved, standard)
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
	pages := extractPages(t, result.PDF)
	visibleText := strings.Join(pages, "\n")
	for _, want := range []string{"Ada Lovelace", "Experienced systems engineer.", "Designed resilient systems."} {
		if !strings.Contains(visibleText, want) {
			t.Errorf("extracted PDF text does not contain %q: %s", want, visibleText)
		}
	}
	if strings.TrimSpace(pages[len(pages)-1]) == "" {
		t.Error("generated PDF has an empty final page")
	}
}

func TestRender_RejectsNilResume(t *testing.T) {
	if _, err := Render(nil, nil); err == nil {
		t.Fatal("expected nil resolved resume to return an error")
	}
}

func TestRender_TextComponentsDoNotOverlap(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace", Phone: "206-555-0142", Email: "ada@example.com"},
		Sections: []string{"skills", "experience"},
		Skills: []resolve.ResolvedSkillGroup{{
			Name: "Platform Engineering", Skills: []string{"Go", "Kubernetes"},
		}},
		Experience: []resolve.ResolvedEmployer{{
			Name: "Acme",
			Roles: []resolve.ResolvedRole{{
				Title: "Principal Reliability Engineer", Accomplishments: []string{"Designed reliable systems."},
			}},
		}},
	}
	result, err := Render(resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	texts := extractTextItems(t, result.PDF)

	phone := findTextItem(t, texts, "206-555-0142")
	email := findTextItem(t, texts, "ada@example.com")
	if abs(phone.Y-email.Y) > 1 || phone.X >= email.X {
		t.Errorf("compact contact fields overlap: phone=%+v email=%+v", phone, email)
	}
	skillHeading := findTextItem(t, texts, "SKILLS")
	groupName := findTextItem(t, texts, "Platform Engineering")
	groupSkills := findTextItem(t, texts, "Go, Kubernetes")
	if !(skillHeading.Y > groupName.Y && groupName.Y > groupSkills.Y) {
		t.Errorf("skill group is not ordered below its heading: heading=%+v name=%+v values=%+v", skillHeading, groupName, groupSkills)
	}
	experienceHeading := findTextItem(t, texts, "EXPERIENCE")
	employer := findTextItem(t, texts, "Acme")
	role := findTextItem(t, texts, "Principal Reliability Engineer")
	accomplishment := findTextItem(t, texts, "Designed reliable systems.")
	if !(experienceHeading.Y > employer.Y && employer.Y > role.Y && role.Y > accomplishment.Y) {
		t.Errorf("experience entry is not vertically ordered: heading=%+v employer=%+v role=%+v accomplishment=%+v", experienceHeading, employer, role, accomplishment)
	}
	for _, text := range texts {
		if strings.Contains(text.S, "GitHub") || strings.Contains(text.S, "LinkedIn") {
			t.Errorf("empty social link was rendered: %q", text.S)
		}
	}
}

func TestRender_SkillGroupNameHasBreathingRoomBeforeSkills(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace"},
		Sections: []string{"skills"},
		Skills: []resolve.ResolvedSkillGroup{{
			Name: "Platform Engineering", Skills: []string{"Go", "AWS"},
		}},
	}
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	result, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	texts := extractTextItems(t, result.PDF)
	name := findTextItem(t, texts, "Platform Engineering")
	skills := findTextItem(t, texts, "Go, AWS")
	pointsPerMillimeter := 72.0 / 25.4
	distanceMM := (name.Y - skills.Y) / pointsPerMillimeter
	minimumMM := standard.Sizes.Body/pointsPerMillimeter + 1
	if distanceMM < minimumMM-0.1 {
		t.Errorf("expected at least 1 mm of additional space after the skill group name, got %v mm total", distanceMM)
	}
}

func TestRender_EntryGapSeparatesContentBlocks(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace"},
		Sections: []string{"skills"},
		Skills: []resolve.ResolvedSkillGroup{
			{Name: "First Group", Skills: []string{"Go"}},
			{Name: "Second Group", Skills: []string{"Kubernetes"}},
		},
	}
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	standard.Spacing.EntryGap = 0
	withoutGap, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	standard.Spacing.EntryGap = 6
	withGap, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}

	withoutTexts := extractTextItems(t, withoutGap.PDF)
	withTexts := extractTextItems(t, withGap.PDF)
	withoutFirst := findTextItem(t, withoutTexts, "First Group")
	withoutSecond := findTextItem(t, withoutTexts, "Second Group")
	withFirst := findTextItem(t, withTexts, "First Group")
	withSecond := findTextItem(t, withTexts, "Second Group")
	withoutDistance := withoutFirst.Y - withoutSecond.Y
	withDistance := withFirst.Y - withSecond.Y
	if withDistance <= withoutDistance {
		t.Errorf("configured entry gap did not increase space between groups: without=%v with=%v", withoutDistance, withDistance)
	}
}

func TestRender_StandardEntryGapAddsOneMillimeterAfterSkills(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace"},
		Sections: []string{"skills"},
		Skills: []resolve.ResolvedSkillGroup{
			{Name: "Platform Engineering", Skills: []string{"Go", "Kubernetes"}},
			{Name: "Reliability", Skills: []string{"OpenTelemetry", "SLO design"}},
		},
	}
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	if standard.Spacing.EntryGap != 3.5 {
		t.Fatalf("expected standard entry gap 3.5 mm, got %v", standard.Spacing.EntryGap)
	}

	comparisonTemplate := *standard
	comparisonTemplate.Spacing.EntryGap -= 1
	withoutExtraGap, err := Render(resolved, &comparisonTemplate)
	if err != nil {
		t.Fatal(err)
	}
	withExtraGap, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	withoutTexts := extractTextItems(t, withoutExtraGap.PDF)
	withTexts := extractTextItems(t, withExtraGap.PDF)
	withoutDistance := findTextItem(t, withoutTexts, "Go, Kubernetes").Y - findTextItem(t, withoutTexts, "Reliability").Y
	withDistance := findTextItem(t, withTexts, "Go, Kubernetes").Y - findTextItem(t, withTexts, "Reliability").Y
	if withDistance-withoutDistance < 0.8 {
		t.Errorf("standard entry gap did not add 1 mm after the skills line: without=%v with=%v", withoutDistance, withDistance)
	}
}

func TestRender_SectionRuleHasGapBeforeContent(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace"},
		Sections: []string{"skills"},
		Skills: []resolve.ResolvedSkillGroup{{
			Name: "Platform Engineering", Skills: []string{"Go"},
		}},
	}
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	showRule := false
	standard.Headings.Rule = &showRule
	withoutRule, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	showRule = true
	withRule, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}

	withoutTexts := extractTextItems(t, withoutRule.PDF)
	withTexts := extractTextItems(t, withRule.PDF)
	withoutGap := findTextItem(t, withoutTexts, "SKILLS").Y - findTextItem(t, withoutTexts, "Platform Engineering").Y
	withGap := findTextItem(t, withTexts, "SKILLS").Y - findTextItem(t, withTexts, "Platform Engineering").Y
	if withGap-withoutGap < 2.5 {
		t.Errorf("section rule does not have the expected 2 mm gap before content: without=%v with=%v", withoutGap, withGap)
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
	if r.template.PageBreaks.PageLimit != 2 {
		t.Errorf("default page limit was not mapped: %d", r.template.PageBreaks.PageLimit)
	}
	if !boolValue(r.template.Headings.Rule, false) || !footerNameEnabled(r.template.Footer.Name) || !footerPageEnabled(r.template.Footer.Page) {
		t.Errorf("heading/footer settings were not mapped: %+v", r.template)
	}
}

func TestRender_PageBoundariesAndPageLimitWarning(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:    schema.Header{Name: "Alex Example"},
		Target:    schema.Target{Company: "Acme", Role: "Engineer"},
		PageLimit: 1,
		Sections:  []string{"impact", "experience"},
		Impact:    make([]resolve.ResolvedImpact, 100),
		Experience: []resolve.ResolvedEmployer{{
			ID: "acme", Name: "Acme",
			Roles: []resolve.ResolvedRole{{
				ID: "role-1", Title: "Engineer", Start: "2020",
				Accomplishments: []string{"First accomplishment stays with the role."},
				Source:          "profile",
			}},
			Source: "profile",
		}},
	}
	for i := range resolved.Impact {
		resolved.Impact[i] = resolve.ResolvedImpact{ID: "impact", Text: "Impact statement demonstrates reliable service delivery."}
	}
	for i := 0; i < 40; i++ {
		resolved.Experience[0].Roles[0].Accomplishments = append(resolved.Experience[0].Roles[0].Accomplishments, "A complete accomplishment sentence that remains intact after pagination.")
	}

	result, err := Render(resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PageCount < 2 || result.PageLimit != 1 {
		t.Fatalf("expected multi-page output over a one-page limit, got %+v", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "page_limit_exceeded" || result.Warnings[0].Section != "experience" {
		t.Fatalf("expected overflow warning to identify experience, got %+v", result.Warnings)
	}

	pages := extractPages(t, result.PDF)
	for i, pageText := range pages {
		wantPageNumber := fmt.Sprintf("Page %d of %d", i+1, result.PageCount)
		if !strings.Contains(pageText, wantPageNumber) {
			t.Errorf("page %d is missing visible page number %q: %s", i+1, wantPageNumber, pageText)
		}
	}
	if strings.TrimSpace(pages[len(pages)-1]) == "" {
		t.Fatal("renderer left an empty trailing page")
	}
	sectionPage := pageContaining(pages, "EXPERIENCE")
	if sectionPage < 2 || !strings.Contains(pages[sectionPage-1], "Acme") || !strings.Contains(pages[sectionPage-1], "Engineer") || !strings.Contains(pages[sectionPage-1], "First accomplishment stays with the role.") {
		t.Fatalf("section heading, employer, role, and first accomplishment should share a page: %v", pages)
	}
	if !strings.Contains(strings.Join(pages, "\n"), "A complete accomplishment sentence that remains intact after pagination.") {
		t.Fatal("long experience entry was lost during pagination")
	}

	second, err := Render(resolved, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.PageCount != result.PageCount || !equalStrings(extractPages(t, second.PDF), pages) {
		t.Fatal("pagination changed for identical input")
	}
}

func extractPages(t *testing.T, data []byte) []string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	pdfFile, reader, err := pdf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer pdfFile.Close()
	pages := make([]string, 0, reader.NumPage())
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		text, err := reader.Page(pageNumber).GetPlainText(nil)
		if err != nil {
			t.Fatal(err)
		}
		pages = append(pages, text)
	}
	return pages
}

func extractTextItems(t *testing.T, data []byte) []pdf.Text {
	t.Helper()
	path := filepath.Join(t.TempDir(), "resume.pdf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	pdfFile, reader, err := pdf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer pdfFile.Close()
	items := reader.Page(1).Content().Text
	runs := make([]pdf.Text, 0, len(items))
	for _, item := range items {
		if len(runs) > 0 {
			last := &runs[len(runs)-1]
			if last.Font == item.Font && last.FontSize == item.FontSize && last.X == item.X && last.Y == item.Y {
				last.S += item.S
				continue
			}
		}
		runs = append(runs, item)
	}
	return runs
}

func findTextItem(t *testing.T, texts []pdf.Text, value string) pdf.Text {
	t.Helper()
	for _, text := range texts {
		if strings.Contains(text.S, value) {
			return text
		}
	}
	t.Fatalf("PDF text %q not found in %+v", value, texts)
	return pdf.Text{}
}

func abs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func pageContaining(pages []string, value string) int {
	for i, page := range pages {
		if strings.Contains(page, value) {
			return i + 1
		}
	}
	return 0
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
