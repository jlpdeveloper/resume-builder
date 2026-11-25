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
	pdftypes "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/goregular"
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
	assertPDFHasEmbeddedUnicodeFont(t, result.PDF)
	pageNumbers := make([]string, result.PageCount)
	for i := range pageNumbers {
		pageNumbers[i] = fmt.Sprint(i + 1)
	}
	if _, ok := pdfURIAnnotations(t, result.PDF, pageNumbers...)["https://example.com/acme"]; !ok {
		t.Error("generated PDF is missing the employer link annotation")
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

func TestRender_HeaderRowsAreLeftAlignedAndCompact(t *testing.T) {
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	resolved := &resolve.Resolved{
		Header: schema.Header{
			Name: "Alex Morgan", Phone: "+1-206-555-0142", Email: "alex.morgan@example.com",
			Location: "Seattle, WA", GitHub: "https://github.com/example",
			LinkedIn: "https://linkedin.com/in/example", Credentials: []string{"U.S. work authorization"},
			Portfolio: []schema.Link{{Label: "Engineering portfolio", URL: "https://example.com/alex"}},
		},
		Target:   schema.Target{Company: "Northstar Analytics", Role: "Staff Platform Engineer", Notes: "Sample target-specific resume."},
		Headline: "Staff platform engineer building reliable infrastructure",
		Summary:  &resolve.ResolvedSummary{Text: "A concise professional summary."},
		Metadata: schema.PDFMeta{Title: "Alex Morgan – Northstar Staff Platform Engineer"},
		Sections: []string{"summary"},
	}
	result, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	texts := extractTextItems(t, result.PDF)
	assertPDFHasTextColor(t, result.PDF, standard.Colors.Link)
	pageHeight := standard.Page.Height * 72 / 25.4
	name := findTextItem(t, texts, "Alex Morgan")
	if name.FontSize != 25 {
		t.Errorf("expected a 25 pt name, got %v pt", name.FontSize)
	}
	if name.Y < pageHeight-50 {
		t.Errorf("name starts too far below the top of the page: y=%v of %v pt", name.Y, pageHeight)
	}
	contentLeft := standard.Margins.Left * 72 / 25.4
	contentRight := (standard.Page.Width - standard.Margins.Right) * 72 / 25.4
	assertPDFLineStartsAt(t, texts, name.Y, contentLeft)

	firstContactRow := findTextItem(t, texts, "+1-206-555-0142 · ")
	secondContactRow := findTextItem(t, texts, "GitHub")
	if got := textAtY(texts, firstContactRow.Y); got != "+1-206-555-0142 · alex.morgan@example.com · Seattle, WA" {
		t.Errorf("unexpected first contact row text %q", got)
	}
	if got := textAtY(texts, secondContactRow.Y); got != "GitHub · LinkedIn · Engineering portfolio" {
		t.Errorf("unexpected second contact row text %q", got)
	}
	for _, item := range []pdf.Text{firstContactRow, secondContactRow} {
		if item.FontSize != 10 {
			t.Errorf("expected 10 pt contact text for %q, got %v pt", item.S, item.FontSize)
		}
	}
	if abs(firstContactRow.X-contentLeft) > 1 || abs(secondContactRow.X-contentLeft) > 1 {
		t.Errorf("contact rows should start at the content margin: first=%+v second=%+v margin=%v", firstContactRow, secondContactRow, contentLeft)
	}
	annotations := headerLinkAnnotations(t, result.PDF)
	for _, uri := range []string{
		"mailto:alex.morgan@example.com",
		"https://github.com/example",
		"https://linkedin.com/in/example",
		"https://example.com/alex",
	} {
		annotation, ok := annotations[uri]
		if !ok {
			t.Errorf("generated PDF is missing header link annotation for %q", uri)
			continue
		}
		if !annotation.Rectangle().Visible() || annotation.Border {
			t.Errorf("header link %q should have a clickable rectangle without a visible border: %+v", uri, annotation)
		}
	}
	emailRect := annotations["mailto:alex.morgan@example.com"].Rectangle()
	githubRect := annotations["https://github.com/example"].Rectangle()
	linkedinRect := annotations["https://linkedin.com/in/example"].Rectangle()
	portfolioRect := annotations["https://example.com/alex"].Rectangle()
	if abs(emailRect.LL.Y-firstContactRow.Y) > 2 ||
		abs(emailRect.LL.X-firstContactRow.X-headerTextWidthPoints("+1-206-555-0142 · ")) > 0.1 ||
		abs(emailRect.Width()-headerTextWidthPoints("alex.morgan@example.com")) > 0.1 || emailRect.UR.X >= contentRight {
		t.Errorf("email annotation should cover the email within the first contact row: rect=%+v row=%+v", emailRect, firstContactRow)
	}
	if abs(githubRect.LL.X-secondContactRow.X) > 0.1 ||
		abs(linkedinRect.LL.X-secondContactRow.X-headerTextWidthPoints("GitHub · ")) > 0.1 ||
		abs(portfolioRect.LL.X-secondContactRow.X-headerTextWidthPoints("GitHub · LinkedIn · ")) > 0.1 ||
		abs(githubRect.Width()-headerTextWidthPoints("GitHub")) > 0.1 ||
		abs(linkedinRect.Width()-headerTextWidthPoints("LinkedIn")) > 0.1 ||
		abs(portfolioRect.Width()-headerTextWidthPoints("Engineering portfolio")) > 0.1 {
		t.Errorf("profile link annotations should follow their labels in the second row: GitHub=%+v LinkedIn=%+v portfolio=%+v row=%+v", githubRect, linkedinRect, portfolioRect, secondContactRow)
	}
	if abs(githubRect.LL.Y-linkedinRect.LL.Y) > 1 || abs(linkedinRect.LL.Y-portfolioRect.LL.Y) > 1 || abs(githubRect.LL.Y-secondContactRow.Y) > 2 {
		t.Errorf("profile link annotations should cover the second contact row: GitHub=%+v LinkedIn=%+v portfolio=%+v row=%+v", githubRect, linkedinRect, portfolioRect, secondContactRow)
	}
	if !(name.Y > firstContactRow.Y && firstContactRow.Y > secondContactRow.Y) {
		t.Errorf("name and contact rows should occupy distinct ordered lines: name=%+v first=%+v second=%+v", name, firstContactRow, secondContactRow)
	}
	if name.Y-firstContactRow.Y < 20 || firstContactRow.Y-secondContactRow.Y < 12 {
		t.Errorf("header rows should have enough vertical separation to avoid overlap: name=%+v first=%+v second=%+v", name, firstContactRow, secondContactRow)
	}

	section := findTextItem(t, texts, "SUMMARY")
	if !(secondContactRow.Y > section.Y) {
		t.Errorf("first section should follow the contact rows in normal flow: contacts=%+v section=%+v", secondContactRow, section)
	}
	if secondContactRow.Y-section.Y > 35 {
		t.Errorf("first section starts too far below the contact rows: gap=%v pt", secondContactRow.Y-section.Y)
	}
	visibleItems := make([]string, len(texts))
	for i, item := range texts {
		visibleItems[i] = item.S
	}
	visibleText := strings.Join(visibleItems, "\n")
	for _, forbidden := range []string{
		resolved.Target.Company, resolved.Target.Role, resolved.Target.Notes,
		resolved.Headline, resolved.Metadata.Title, "U.S. work authorization",
	} {
		if forbidden == "" {
			continue
		}
		if strings.Contains(visibleText, forbidden) {
			t.Errorf("header-only target or credential text %q should not be visible in the resume", forbidden)
		}
	}
}

func headerLinkAnnotations(t *testing.T, pdfBytes []byte) map[string]model.LinkAnnotation {
	t.Helper()
	return pdfURIAnnotations(t, pdfBytes, "1")
}

func assertPDFHasTextColor(t *testing.T, pdfBytes []byte, hexColor string) {
	t.Helper()
	color := parseColor(hexColor, nil)
	expected := []byte(fmt.Sprintf("%.3f %.3f %.3f rg", float64(color.Red)/255, float64(color.Green)/255, float64(color.Blue)/255))
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdfBytes), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateContext(context.Background(), ctx); err != nil {
		t.Fatal(err)
	}
	page, _, _, err := ctx.PageDict(context.Background(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := ctx.Dereference(page["Contents"])
	if err != nil {
		t.Fatal(err)
	}
	var streams []pdftypes.StreamDict
	switch content := contents.(type) {
	case pdftypes.StreamDict:
		streams = append(streams, content)
	case *pdftypes.StreamDict:
		streams = append(streams, *content)
	case pdftypes.Array:
		for _, object := range content {
			stream, err := ctx.Dereference(object)
			if err != nil {
				t.Fatal(err)
			}
			switch stream := stream.(type) {
			case pdftypes.StreamDict:
				streams = append(streams, stream)
			case *pdftypes.StreamDict:
				streams = append(streams, *stream)
			}
		}
	default:
		t.Fatalf("unexpected page content type %T", contents)
	}
	for i := range streams {
		if err := streams[i].Decode(); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(streams[i].Content, expected) {
			return
		}
	}
	t.Errorf("generated PDF is missing configured link text color %q", expected)
}

func pdfURIAnnotations(t *testing.T, pdfBytes []byte, pages ...string) map[string]model.LinkAnnotation {
	t.Helper()
	pageAnnotations, err := api.Annotations(context.Background(), bytes.NewReader(pdfBytes), pages, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	links := make(map[string]model.LinkAnnotation)
	for _, page := range pageAnnotations {
		for _, annotation := range page[model.AnnLink].Map {
			link, ok := annotation.(model.LinkAnnotation)
			if !ok {
				if pointer, pointerOK := annotation.(*model.LinkAnnotation); pointerOK {
					link = *pointer
					ok = true
				}
			}
			if ok {
				links[link.URI] = link
			}
		}
	}
	return links
}

func assertPDFHasEmbeddedUnicodeFont(t *testing.T, pdfBytes []byte) {
	t.Helper()
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdfBytes), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if err := api.ValidateContext(context.Background(), ctx); err != nil {
		t.Fatal(err)
	}
	page, _, _, err := ctx.PageDict(context.Background(), 1, true)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := ctx.DereferenceDict(page["Resources"])
	if err != nil {
		t.Fatal(err)
	}
	fonts, err := ctx.DereferenceDict(resources["Font"])
	if err != nil {
		t.Fatal(err)
	}

	hasToUnicode := false
	hasFontFile2 := false
	checkFontDescriptor := func(font pdftypes.Dict) {
		fontDescriptor, ok := font.Find("FontDescriptor")
		if !ok {
			return
		}
		descriptor, err := ctx.DereferenceDict(fontDescriptor)
		if err != nil {
			t.Errorf("read embedded font descriptor: %v", err)
			return
		}
		hasFontFile2 = hasFontFile2 || descriptor.HasEntry("FontFile2")
	}
	for _, fontObject := range fonts {
		font, err := ctx.DereferenceDict(fontObject)
		if err != nil {
			t.Errorf("read font dictionary: %v", err)
			continue
		}
		hasToUnicode = hasToUnicode || font.HasEntry("ToUnicode")
		checkFontDescriptor(font)
		for _, descendantObject := range font.ArrayEntry("DescendantFonts") {
			descendant, err := ctx.DereferenceDict(descendantObject)
			if err != nil {
				t.Errorf("read descendant font dictionary: %v", err)
				continue
			}
			checkFontDescriptor(descendant)
		}
	}
	if !hasToUnicode {
		t.Error("generated PDF has no ToUnicode font mapping")
	}
	if !hasFontFile2 {
		t.Error("generated PDF has no embedded TrueType font")
	}
}

func headerTextWidthPoints(value string) float64 {
	pdf := gofpdf.New("P", "mm", "Letter", "")
	pdf.AddUTF8FontFromBytes(defaultFontFamily, "", goregular.TTF)
	pdf.SetFont(defaultFontFamily, "", 10)
	return pdf.GetStringWidth(value) * 72 / 25.4
}

func assertPDFLineStartsAt(t *testing.T, texts []pdf.Text, y, expectedX float64) {
	t.Helper()
	minX := 1e9
	found := false
	for _, item := range texts {
		if abs(item.Y-y) > 1 {
			continue
		}
		found = true
		if item.X < minX {
			minX = item.X
		}
	}
	if !found || abs(minX-expectedX) > 1 {
		t.Errorf("PDF line does not start at the content margin: x=%v, expected=%v", minX, expectedX)
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

	contactRow := findTextItem(t, texts, "206-555-0142 · ")
	if got := textAtY(texts, contactRow.Y); got != "206-555-0142 · ada@example.com" {
		t.Errorf("unexpected contact row text %q", got)
	}
	name := findTextItem(t, texts, "Ada Lovelace")
	if !(name.Y > contactRow.Y) {
		t.Errorf("contact row should occupy a separate line below the name: name=%+v contacts=%+v", name, contactRow)
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

func TestRender_ImpactSpacingDoesNotUseEntryGap(t *testing.T) {
	resolved := &resolve.Resolved{
		Header:   schema.Header{Name: "Ada Lovelace"},
		Sections: []string{"impact"},
		Impact: []resolve.ResolvedImpact{
			{Text: "First impact statement."},
			{Text: "Second impact statement."},
		},
	}
	standard, err := resumeTemplate.Standard()
	if err != nil {
		t.Fatal(err)
	}
	withoutEntryGap := *standard
	withoutEntryGap.Spacing.EntryGap = 0
	configuredImpactGap := *standard
	configuredImpactGap.Spacing.ImpactGap = 2

	withResult, err := Render(resolved, standard)
	if err != nil {
		t.Fatal(err)
	}
	withoutResult, err := Render(resolved, &withoutEntryGap)
	if err != nil {
		t.Fatal(err)
	}
	configuredImpactResult, err := Render(resolved, &configuredImpactGap)
	if err != nil {
		t.Fatal(err)
	}
	withTexts := extractTextItems(t, withResult.PDF)
	withoutTexts := extractTextItems(t, withoutResult.PDF)
	configuredImpactTexts := extractTextItems(t, configuredImpactResult.PDF)
	withDistance := findTextItem(t, withTexts, "First impact statement").Y - findTextItem(t, withTexts, "Second impact statement").Y
	withoutDistance := findTextItem(t, withoutTexts, "First impact statement").Y - findTextItem(t, withoutTexts, "Second impact statement").Y
	if abs(withDistance-withoutDistance) > 0.1 {
		t.Errorf("impact spacing should not include the general entry gap: with=%v without=%v", withDistance, withoutDistance)
	}
	configuredImpactDistance := findTextItem(t, configuredImpactTexts, "First impact statement").Y - findTextItem(t, configuredImpactTexts, "Second impact statement").Y
	wantImpactGapPoints := 2 * 72.0 / 25.4
	if abs(configuredImpactDistance-withDistance-wantImpactGapPoints) > 0.1 {
		t.Errorf("impactGap should add 2 mm between impact statements: default=%v configured=%v", withDistance, configuredImpactDistance)
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
	if r.regular != "ResumeSans" || r.bold != "ResumeSans" || r.nameSize != 25 || r.contactSize != 10 {
		t.Errorf("font settings were not mapped: %+v", r)
	}
	if r.headingAlign != align.Left || !boolValue(standard.Header.Compact, false) {
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

func textAtY(texts []pdf.Text, y float64) string {
	var line strings.Builder
	for _, text := range texts {
		if abs(text.Y-y) < 0.1 {
			line.WriteString(text.S)
		}
	}
	return line.String()
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
