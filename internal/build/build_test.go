package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func buildFixture(t *testing.T, impactCount, pageLimit int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	profileLines := []string{
		"schema: resume/v1", "kind: profile", "id: p1",
		"header:", "  name: Alex Example",
	}
	if impactCount == 0 {
		profileLines = append(profileLines, "summaries:", "  - id: summary-1", "    text: Experienced engineer.")
	} else {
		profileLines = append(profileLines, "impact:")
		for i := 0; i < impactCount; i++ {
			profileLines = append(profileLines, fmt.Sprintf("  - id: impact-%d", i), "    text: Improved service reliability across production.")
		}
	}
	writeBuildYAML(t, dir, "profile.yml", profileLines...)
	templateLines := []string{
		"schema: resume/v1", "kind: template", "id: t1",
		"page:", "  width: 215.9", "  height: 279.4",
		"fonts:", "  regular: ResumeSans", "  bold: ResumeSans",
		"pageBreaks:", fmt.Sprintf("  pageLimit: %d", pageLimit),
	}
	writeBuildYAML(t, dir, "template.yml", templateLines...)
	writeBuildYAML(t, dir, "base.yml",
		"schema: resume/v1", "kind: base", "id: b1", "profile: profile.yml", "template: template.yml",
	)
	resumeLines := []string{
		"schema: resume/v1", "kind: resume", "id: r1",
		"profile: profile.yml", "base: base.yml", "template: template.yml",
		"target:", "  company: Acme", "  role: Platform Engineer", "  url: https://example.com/jobs",
	}
	if impactCount == 0 {
		resumeLines = append(resumeLines, "summary:", "  id: summary-1", "sections: [summary]")
	} else {
		resumeLines = append(resumeLines, "impact:")
		for i := 0; i < impactCount; i++ {
			resumeLines = append(resumeLines, fmt.Sprintf("  - id: impact-%d", i))
		}
		resumeLines = append(resumeLines, "sections: [impact]")
	}
	resumePath := writeBuildYAML(t, dir, "resume.yml", resumeLines...)
	return resumePath, dir
}

func writeBuildYAML(t *testing.T, dir, name string, lines ...string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBuild_WritesPDFAndResolvedManifest(t *testing.T) {
	resumePath, dir := buildFixture(t, 0, 2)
	pdfPath := filepath.Join(dir, "out", "resume.pdf")
	manifestPath := filepath.Join(dir, "out", "resume.json")
	result := Build(resumePath, Options{PDFPath: pdfPath, ManifestPath: manifestPath, CLIVersion: "0.1.0"})
	if !result.Success {
		t.Fatalf("build failed: %+v", result)
	}
	if result.PageCount < 1 || result.PDFPath != pdfPath || result.ManifestPath != manifestPath {
		t.Fatalf("unexpected build result: %+v", result)
	}
	pdfData, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(pdfData), "%PDF-") {
		t.Fatal("output file is not a PDF")
	}
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ResumeID != "r1" || manifest.SchemaVersion != "resume/v1" || manifest.ProfileID != "p1" || manifest.BaseID != "b1" || manifest.TemplateID != "t1" {
		t.Errorf("unexpected manifest IDs: %+v", manifest)
	}
	if manifest.Target.Company != "Acme" || manifest.Target.Role != "Platform Engineer" || manifest.Target.URL != "https://example.com/jobs" {
		t.Errorf("unexpected manifest target: %+v", manifest.Target)
	}
	if len(manifest.Sections) != 1 || manifest.Sections[0].Name != "summary" || len(manifest.Sections[0].Items) != 1 {
		t.Fatalf("unexpected manifest sections: %+v", manifest.Sections)
	}
	item := manifest.Sections[0].Items[0]
	if item.ID != "summary-1" || item.SourceKind != "profile" || item.SourceFile != filepath.Join(dir, "profile.yml") {
		t.Errorf("manifest item omitted its ID or source: %+v", item)
	}
	if len(manifest.InputHashes) != 4 || manifest.InputSHA256 == "" || manifest.InputSHA256 != result.Fingerprints.InputSHA256 || manifest.OutputSHA256 == "" || manifest.PageCount != result.PageCount || manifest.CLIVersion != "0.1.0" || manifest.GeneratedAt.IsZero() {
		t.Errorf("manifest omitted fingerprints or build metadata: %+v", manifest)
	}
	for _, input := range manifest.InputHashes {
		data, err := os.ReadFile(input.Path)
		if err != nil {
			t.Fatal(err)
		}
		inputHash := sha256.Sum256(data)
		if input.SHA256 != hex.EncodeToString(inputHash[:]) {
			t.Errorf("input fingerprint mismatch for %s", input.Path)
		}
	}
	hash := sha256.Sum256(pdfData)
	if manifest.OutputSHA256 != hex.EncodeToString(hash[:]) || manifest.OutputSHA256 != result.Fingerprints.OutputSHA256 {
		t.Errorf("output fingerprint mismatch: manifest=%s result=%s", manifest.OutputSHA256, result.Fingerprints.OutputSHA256)
	}
	if len(manifest.Warnings) != 0 {
		t.Errorf("expected no warnings, got %+v", manifest.Warnings)
	}
}

func TestBuild_ReproducibleWithFixedTimestamp(t *testing.T) {
	resumePath, dir := buildFixture(t, 0, 2)
	pdfPath := filepath.Join(dir, "resume.pdf")
	manifestPath := filepath.Join(dir, "resume.manifest.json")
	generatedAt := time.Date(2024, time.January, 2, 3, 4, 5, 0, time.FixedZone("test", -5*60*60))
	options := Options{PDFPath: pdfPath, ManifestPath: manifestPath, CLIVersion: "test-version", Timestamp: generatedAt}

	first := Build(resumePath, options)
	if !first.Success {
		t.Fatalf("first build failed: %+v", first)
	}
	firstPDF, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	second := Build(resumePath, options)
	if !second.Success {
		t.Fatalf("second build failed: %+v", second)
	}
	secondPDF, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	secondInfo, err := api.PDFInfo(context.Background(), bytes.NewReader(secondPDF), filepath.Base(pdfPath), nil, false, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprints.InputSHA256 == "" || first.Fingerprints.InputSHA256 != second.Fingerprints.InputSHA256 {
		t.Fatalf("input fingerprints differ: %q and %q", first.Fingerprints.InputSHA256, second.Fingerprints.InputSHA256)
	}
	if first.Fingerprints.OutputSHA256 == "" || first.Fingerprints.OutputSHA256 != second.Fingerprints.OutputSHA256 || !bytes.Equal(firstPDF, secondPDF) {
		t.Fatalf("fixed-timestamp builds were not byte-for-byte reproducible: %q and %q", first.Fingerprints.OutputSHA256, second.Fingerprints.OutputSHA256)
	}

	otherResumePath, otherDir := buildFixture(t, 0, 2)
	other := Build(otherResumePath, Options{
		PDFPath: filepath.Join(otherDir, "resume.pdf"), ManifestPath: filepath.Join(otherDir, "resume.manifest.json"),
		CLIVersion: options.CLIVersion, Timestamp: options.Timestamp,
	})
	if !other.Success {
		t.Fatalf("build in a different directory failed: %+v", other)
	}
	if first.Fingerprints.InputSHA256 != other.Fingerprints.InputSHA256 {
		t.Errorf("identical inputs in different directories have different fingerprints: %q and %q", first.Fingerprints.InputSHA256, other.Fingerprints.InputSHA256)
	}
	if first.Fingerprints.OutputSHA256 != other.Fingerprints.OutputSHA256 {
		t.Errorf("identical inputs in different directories have different PDF fingerprints: %q and %q", first.Fingerprints.OutputSHA256, other.Fingerprints.OutputSHA256)
	}

	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	if !manifest.GeneratedAt.Equal(generatedAt.UTC()) || manifest.CLIVersion != options.CLIVersion || manifest.SchemaVersion != "resume/v1" || manifest.InputSHA256 != first.Fingerprints.InputSHA256 {
		t.Fatalf("manifest did not preserve reproducible build metadata: %+v", manifest)
	}
	wantPDFTimestamp := generatedAt.UTC().Format("20060102150405")
	if !strings.Contains(secondInfo.CreationDate, wantPDFTimestamp) {
		t.Errorf("PDF creation date %q does not match fixed timestamp %s", secondInfo.CreationDate, generatedAt.UTC().Format(time.RFC3339))
	}
	if !strings.Contains(secondInfo.ModificationDate, wantPDFTimestamp) {
		t.Errorf("PDF modification date %q does not match fixed timestamp %s", secondInfo.ModificationDate, generatedAt.UTC().Format(time.RFC3339))
	}
}

func TestParseBuildArgs_Timestamp(t *testing.T) {
	want, err := time.Parse(time.RFC3339, "2024-01-02T03:04:05-05:00")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"resume.yml", "--timestamp", "2024-01-02T03:04:05-05:00"},
		{"resume.yml", "--timestamp=2024-01-02T03:04:05-05:00"},
	} {
		_, options, err := parseBuildArgs(args)
		if err != nil {
			t.Fatalf("parseBuildArgs(%q): %v", args, err)
		}
		if !options.Timestamp.Equal(want) || options.Timestamp.Location() != time.UTC {
			t.Errorf("parseBuildArgs(%q) timestamp = %s (%s), want %s in UTC", args, options.Timestamp, options.Timestamp.Location(), want.UTC())
		}
	}
	for _, args := range [][]string{
		{"resume.yml", "--timestamp"},
		{"resume.yml", "--timestamp="},
		{"resume.yml", "--timestamp", "yesterday"},
	} {
		if _, _, err := parseBuildArgs(args); err == nil {
			t.Errorf("parseBuildArgs(%q) unexpectedly succeeded", args)
		}
	}
}

func TestBuild_ValidationFailureDoesNotWriteOutputs(t *testing.T) {
	resumePath, dir := buildFixture(t, 0, 2)
	if err := os.WriteFile(resumePath, []byte("schema: resume/v1\nkind: resume\nid: r1\nprofile: profile.yml\ntarget:\n  company: Acme\n  role: Engineer\nunknownField: true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(dir, "resume.pdf")
	manifestPath := filepath.Join(dir, "resume.manifest.json")
	result := Build(resumePath, Options{PDFPath: pdfPath, ManifestPath: manifestPath})
	if result.Success || len(result.Diagnostics) == 0 {
		t.Fatalf("expected validation failure, got %+v", result)
	}
	for _, path := range []string{pdfPath, manifestPath} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("output %q should not be written before validation succeeds", path)
		}
	}
}

func TestBuild_PageLimitWarningIsManifested(t *testing.T) {
	resumePath, dir := buildFixture(t, 150, 1)
	result := Build(resumePath, Options{
		PDFPath:      filepath.Join(dir, "resume.pdf"),
		ManifestPath: filepath.Join(dir, "resume.manifest.json"),
	})
	if !result.Success || result.PageCount <= 1 {
		t.Fatalf("expected successful multi-page build, got %+v", result)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "page_limit_exceeded" || result.Warnings[0].Section != "impact" {
		t.Fatalf("expected a page-limit warning for impact, got %+v", result.Warnings)
	}
	data, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Warnings) != 1 || manifest.Warnings[0].Section != "impact" {
		t.Errorf("manifest did not preserve the page-limit warning: %+v", manifest.Warnings)
	}
}

func TestRun_JSONBuildResultAndOutputFlags(t *testing.T) {
	resumePath, dir := buildFixture(t, 0, 2)
	stdout := &strings.Builder{}
	stderr := &strings.Builder{}
	pdfPath := filepath.Join(dir, "explicit.pdf")
	manifestPath := filepath.Join(dir, "explicit.json")
	code := Run([]string{resumePath, "--output", pdfPath, "--manifest=" + manifestPath}, stdout, stderr, true, "test-version")
	if code != 0 {
		t.Fatalf("expected successful command, got %d: %s", code, stderr.String())
	}
	var result BuildResult
	if err := json.Unmarshal([]byte(stdout.String()), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.PDFPath != pdfPath || result.ManifestPath != manifestPath {
		t.Fatalf("unexpected structured result: %+v", result)
	}
}

func TestBuild_RejectsOutputPathThatWouldOverwriteInput(t *testing.T) {
	resumePath, _ := buildFixture(t, 0, 2)
	result := Build(resumePath, Options{PDFPath: resumePath})
	if result.Success || !strings.Contains(result.Error, "must not overwrite") {
		t.Fatalf("expected output collision to fail, got %+v", result)
	}
}

func TestSectionItemsIncludeContentAndSource(t *testing.T) {
	resolved := &resolve.Resolved{
		ProfilePath: "profile.yml", BasePath: "base.yml", ResumePath: "resume.yml",
		Experience: []resolve.ResolvedEmployer{{
			ID: "company-1", Name: "Acme", Location: "Remote", Link: "https://example.com",
			Source: "base",
			Roles: []resolve.ResolvedRole{{
				ID: "role-1", Title: "Engineer", Start: "2020", End: "2022",
				Accomplishments: []string{"Built a service."}, Source: "base",
			}},
		}},
		Projects:  []schema.Project{{ID: "project-1", Name: "API", Role: "Maintainer", Link: "https://example.com/api", Tech: "Go", Accomplishments: []string{"Shipped a service."}}},
		Education: []schema.Education{{ID: "education-1", Institution: "Example University", Degree: "BS", Field: "Computer Science", Graduation: "2020"}},
		Certs:     []schema.Certification{{ID: "cert-1", Name: "Go Certified", Issuer: "Example Org", CredentialURL: "https://example.com/cert"}},
	}

	experience := sectionItems("experience", resolved)
	if len(experience) != 3 || experience[0].SourceFile != "base.yml" || experience[1].ID != "role-1" || experience[2].SourceFile != "base.yml" {
		t.Fatalf("experience IDs or source files missing: %+v", experience)
	}
	projects := sectionItems("projects", resolved)
	if projects[0].Details["link"] != "https://example.com/api" || projects[0].Details["tech"] != "Go" {
		t.Errorf("project details missing: %+v", projects[0])
	}
	education := sectionItems("education", resolved)
	if education[0].Details["field"] != "Computer Science" || education[0].SourceFile != "profile.yml" {
		t.Errorf("education details or source missing: %+v", education[0])
	}
	certifications := sectionItems("certifications", resolved)
	if certifications[0].Details["credential_url"] != "https://example.com/cert" {
		t.Errorf("certification details missing: %+v", certifications[0])
	}
}
