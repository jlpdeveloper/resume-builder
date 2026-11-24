package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"github.com/jlpdeveloper/resume-builder/internal/validate"
)

// Options controls the output files and build metadata.
type Options struct {
	PDFPath      string
	ManifestPath string
	CLIVersion   string
}

// FileHash fingerprints one YAML input file.
type FileHash struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// Fingerprints contains input and output checksums for a build.
type Fingerprints struct {
	Inputs       []FileHash `json:"inputs"`
	OutputSHA256 string     `json:"output_sha256"`
}

// Target contains the job metadata stored in a build manifest.
type Target struct {
	Company string `json:"company"`
	Role    string `json:"role"`
	URL     string `json:"url,omitempty"`
}

// ManifestItem is one resolved content entry and its source file.
type ManifestItem struct {
	ID         string         `json:"id,omitempty"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name,omitempty"`
	Text       string         `json:"text,omitempty"`
	Values     []string       `json:"values,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	SourceKind string         `json:"source_kind"`
	SourceFile string         `json:"source_file"`
}

// ManifestSection contains resolved items in output order.
type ManifestSection struct {
	Name  string         `json:"name"`
	Items []ManifestItem `json:"items"`
}

// Manifest records the resolved résumé, source fingerprints, and PDF output.
type Manifest struct {
	ResumeID      string            `json:"resume_id"`
	SchemaVersion string            `json:"schema_version"`
	ProfileID     string            `json:"profile_id"`
	BaseID        string            `json:"base_id,omitempty"`
	TemplateID    string            `json:"template_id"`
	Target        Target            `json:"target"`
	Sections      []ManifestSection `json:"sections"`
	InputHashes   []FileHash        `json:"input_hashes"`
	OutputSHA256  string            `json:"output_sha256"`
	PDFPath       string            `json:"pdf_path"`
	PageCount     int               `json:"page_count"`
	CLIVersion    string            `json:"cli_version"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Warnings      []Warning         `json:"warnings"`
}

// BuildResult is the structured result returned by the build command.
type BuildResult struct {
	Success      bool                  `json:"success"`
	PDFPath      string                `json:"pdf_path,omitempty"`
	ManifestPath string                `json:"manifest_path,omitempty"`
	PageCount    int                   `json:"page_count,omitempty"`
	Warnings     []Warning             `json:"warnings"`
	Fingerprints Fingerprints          `json:"fingerprints"`
	Diagnostics  []validate.Diagnostic `json:"diagnostics,omitempty"`
	Error        string                `json:"error,omitempty"`
}

type resumeFiles struct {
	resume   *resolve.Document
	profile  *resolve.Document
	base     *resolve.Document
	template *resolve.Document
}

// Build validates, resolves, renders, and writes the PDF and content manifest.
func Build(path string, options Options) BuildResult {
	result := BuildResult{Warnings: []Warning{}, Fingerprints: Fingerprints{Inputs: []FileHash{}}}
	pdfPath, manifestPath, err := outputPaths(path, options)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.PDFPath, result.ManifestPath = pdfPath, manifestPath

	validationResult := validate.Validate(path)
	if !validationResult.Valid {
		result.Error = "resume validation failed"
		result.Diagnostics = validationResult.Errors
		result.Warnings = validationWarnings(validationResult.Warnings)
		return result
	}

	graph, err := (&resolve.Resolver{}).Resolve(path)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	files := indexFiles(graph)
	if files.resume == nil || files.profile == nil {
		result.Error = "build requires a resume and its profile"
		return result
	}
	if pathClashes(pdfPath, manifestPath, graph.Documents) {
		result.Error = "output paths must not overwrite an input file or each other"
		return result
	}
	resolved, err := resolve.ResolveContent(graph)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	inputHashes := hashInputs(graph.Documents)
	version := options.CLIVersion
	if version == "" {
		version = "unknown"
	}
	for _, warning := range validationWarnings(validationResult.Warnings) {
		result.Warnings = append(result.Warnings, warning)
	}
	rendered, err := Render(resolved, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.PageCount = rendered.PageCount
	result.Warnings = append(result.Warnings, rendered.Warnings...)
	result.Fingerprints.Inputs = inputHashes
	result.Fingerprints.OutputSHA256 = hashBytes(rendered.PDF)

	manifest := makeManifest(files, resolved, inputHashes, result.Fingerprints.OutputSHA256, pdfPath, rendered.PageCount, version, result.Warnings)
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		result.Error = fmt.Sprintf("encode manifest: %v", err)
		return result
	}
	manifestData = append(manifestData, '\n')
	if err := writeOutput(pdfPath, rendered.PDF); err != nil {
		result.Error = fmt.Sprintf("write PDF: %v", err)
		return result
	}
	if err := writeOutput(manifestPath, manifestData); err != nil {
		result.Error = fmt.Sprintf("write manifest: %v", err)
		return result
	}
	result.Success = true
	return result
}

// Run implements the build command.
func Run(args []string, stdout, stderr io.Writer, jsonOutput bool, cliVersion string) int {
	path, options, err := parseBuildArgs(args)
	if err != nil {
		result := BuildResult{Warnings: []Warning{}, Fingerprints: Fingerprints{Inputs: []FileHash{}}, Error: err.Error()}
		return writeBuildResult(stdout, stderr, result, jsonOutput)
	}
	options.CLIVersion = cliVersion
	result := Build(path, options)
	return writeBuildResult(stdout, stderr, result, jsonOutput)
}

func writeBuildResult(stdout, stderr io.Writer, result BuildResult, jsonOutput bool) int {
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "write build result: %v\n", err)
			return 1
		}
	} else if result.Success {
		fmt.Fprintf(stdout, "Built %s (%d pages)\nManifest: %s\n", result.PDFPath, result.PageCount, result.ManifestPath)
		for _, warning := range result.Warnings {
			fmt.Fprintf(stdout, "Warning: %s\n", warning.Message)
		}
	} else {
		fmt.Fprintf(stderr, "Build failed: %s\n", result.Error)
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stderr, "%s:%d: %s: %s\n", diagnostic.Path, diagnostic.Line, diagnostic.Field, diagnostic.Message)
		}
	}
	if result.Success {
		return 0
	}
	return 1
}

func parseBuildArgs(args []string) (string, Options, error) {
	if len(args) == 0 {
		return "", Options{}, fmt.Errorf("Usage: resume build PATH [--output PDF] [--manifest JSON]")
	}
	path := args[0]
	if strings.HasPrefix(path, "-") || strings.TrimSpace(path) == "" {
		return "", Options{}, fmt.Errorf("Usage: resume build PATH [--output PDF] [--manifest JSON]")
	}
	options := Options{}
	for i := 1; i < len(args); i++ {
		argument := args[i]
		var value string
		switch {
		case argument == "--output" || argument == "-o":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", Options{}, fmt.Errorf("%s requires a path", argument)
			}
			i++
			value = args[i]
			options.PDFPath = value
		case strings.HasPrefix(argument, "--output="):
			options.PDFPath = strings.TrimPrefix(argument, "--output=")
		case argument == "--manifest":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", Options{}, fmt.Errorf("--manifest requires a path")
			}
			i++
			options.ManifestPath = args[i]
		case strings.HasPrefix(argument, "--manifest="):
			options.ManifestPath = strings.TrimPrefix(argument, "--manifest=")
		default:
			return "", Options{}, fmt.Errorf("unknown build option %q", argument)
		}
	}
	if options.PDFPath == "" && hasBuildOption(args, "--output=") {
		return "", Options{}, fmt.Errorf("--output requires a path")
	}
	if options.ManifestPath == "" && hasBuildOption(args, "--manifest=") {
		return "", Options{}, fmt.Errorf("--manifest requires a path")
	}
	return path, options, nil
}

func hasBuildOption(args []string, prefix string) bool {
	for _, argument := range args {
		if strings.HasPrefix(argument, prefix) {
			return true
		}
	}
	return false
}

func outputPaths(input string, options Options) (string, string, error) {
	pdfPath := options.PDFPath
	if pdfPath == "" {
		pdfPath = strings.TrimSuffix(input, filepath.Ext(input)) + ".pdf"
	}
	manifestPath := options.ManifestPath
	if manifestPath == "" {
		manifestPath = strings.TrimSuffix(pdfPath, filepath.Ext(pdfPath)) + ".manifest.json"
	}
	pdfPath, err := filepath.Abs(pdfPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve PDF output path: %w", err)
	}
	manifestPath, err = filepath.Abs(manifestPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve manifest output path: %w", err)
	}
	return filepath.Clean(pdfPath), filepath.Clean(manifestPath), nil
}

func indexFiles(graph *resolve.Graph) resumeFiles {
	var files resumeFiles
	for _, doc := range graph.Documents {
		switch doc.Kind {
		case schema.KindResume:
			files.resume = doc
		case schema.KindProfile:
			files.profile = doc
		case schema.KindBase:
			files.base = doc
		case schema.KindTemplate:
			files.template = doc
		}
	}
	return files
}

func pathClashes(pdfPath, manifestPath string, documents []*resolve.Document) bool {
	if samePath(pdfPath, manifestPath) {
		return true
	}
	for _, doc := range documents {
		if samePath(pdfPath, doc.Path) || samePath(manifestPath, doc.Path) {
			return true
		}
	}
	return false
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return strings.EqualFold(left, right)
}

func hashInputs(documents []*resolve.Document) []FileHash {
	inputs := make([]FileHash, 0, len(documents))
	for _, doc := range documents {
		inputs = append(inputs, FileHash{Path: doc.Path, SHA256: hashBytes(doc.Raw)})
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].Path < inputs[j].Path })
	return inputs
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func makeManifest(files resumeFiles, resolved *resolve.Resolved, inputs []FileHash, outputHash, pdfPath string, pageCount int, cliVersion string, warnings []Warning) Manifest {
	templateID := "standard"
	if files.template != nil {
		templateID = files.template.Envelope.ID
	}
	manifest := Manifest{
		ResumeID:      files.resume.Envelope.ID,
		SchemaVersion: files.resume.Envelope.Schema,
		ProfileID:     files.profile.Envelope.ID,
		TemplateID:    templateID,
		Target:        Target{Company: resolved.Target.Company, Role: resolved.Target.Role, URL: resolved.Target.URL},
		Sections:      make([]ManifestSection, 0, len(resolved.Sections)),
		InputHashes:   inputs,
		OutputSHA256:  outputHash,
		PDFPath:       pdfPath,
		PageCount:     pageCount,
		CLIVersion:    cliVersion,
		GeneratedAt:   time.Now().UTC(),
		Warnings:      append([]Warning{}, warnings...),
	}
	if files.base != nil {
		manifest.BaseID = files.base.Envelope.ID
	}
	for _, section := range resolved.Sections {
		manifest.Sections = append(manifest.Sections, ManifestSection{Name: section, Items: sectionItems(section, resolved)})
	}
	return manifest
}

func sectionItems(section string, resolved *resolve.Resolved) []ManifestItem {
	items := make([]ManifestItem, 0)
	switch section {
	case "summary":
		if resolved.Summary != nil {
			items = append(items, manifestItem(resolved.Summary.ID, "summary", "", resolved.Summary.Text, nil, nil, resolved.Summary.Source, resolved))
		}
	case "impact":
		for _, impact := range resolved.Impact {
			items = append(items, manifestItem(impact.ID, "impact", "", impact.Text, nil, nil, impact.Source, resolved))
		}
	case "skills":
		for _, group := range resolved.Skills {
			items = append(items, manifestItem(group.ID, "skill_group", group.Name, "", group.Skills, nil, group.Source, resolved))
		}
	case "experience":
		for _, employer := range resolved.Experience {
			items = append(items, manifestItem(employer.ID, "employer", employer.Name, "", nil, map[string]any{
				"location": employer.Location, "link": employer.Link,
			}, employer.Source, resolved))
			for _, role := range employer.Roles {
				roleText := role.Title + " (" + role.Start
				if role.End != "" {
					roleText += "–" + role.End
				}
				roleText += ")"
				items = append(items, manifestItem(role.ID, "role", roleText, "", nil, map[string]any{
					"title": role.Title, "start": role.Start, "end": role.End,
				}, role.Source, resolved))
				for i, accomplishment := range role.Accomplishments {
					id := fmt.Sprintf("%s-%s-accomplishment-%d", employer.ID, role.ID, i+1)
					items = append(items, manifestItem(id, "accomplishment", "", accomplishment, nil, nil, role.Source, resolved))
				}
			}
		}
	case "projects":
		for _, project := range resolved.Projects {
			items = append(items, manifestItem(project.ID, "project", project.Name, project.Role, nil, map[string]any{
				"start": project.Start, "end": project.End, "link": project.Link,
				"tech": project.Tech, "accomplishments": project.Accomplishments,
			}, "profile", resolved))
		}
	case "education":
		for _, education := range resolved.Education {
			items = append(items, manifestItem(education.ID, "education", education.Institution, education.Degree, nil, map[string]any{
				"field": education.Field, "graduation": education.Graduation, "honors": education.Honors,
			}, "profile", resolved))
		}
	case "certifications":
		for _, cert := range resolved.Certs {
			items = append(items, manifestItem(cert.ID, "certification", cert.Name, cert.Issuer, nil, map[string]any{
				"date_earned": cert.DateEarned, "expiration": cert.Expiration,
				"credential_url": cert.CredentialURL,
			}, "profile", resolved))
		}
	}
	return items
}

func manifestItem(id, kind, name, text string, values []string, details map[string]any, source string, resolved *resolve.Resolved) ManifestItem {
	return ManifestItem{
		ID: id, Kind: kind, Name: name, Text: text, Values: values, Details: details,
		SourceKind: source, SourceFile: sourcePath(resolved, source),
	}
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

func validationWarnings(warnings []validate.Diagnostic) []Warning {
	result := make([]Warning, 0, len(warnings))
	for _, warning := range warnings {
		result = append(result, Warning{Code: "validation_warning", Field: warning.Field, Message: warning.Message, Path: warning.Path, Line: warning.Line})
	}
	return result
}

func writeOutput(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
