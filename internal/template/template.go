package template

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"gopkg.in/yaml.v3"
)

//go:embed standard.yml
var files embed.FS

type templateFile struct {
	schema.Envelope `yaml:",inline"`
	schema.Template `yaml:",inline"`
}

// Standard returns the embedded standard template.
func Standard() (*schema.Template, error) {
	data, err := files.ReadFile("standard.yml")
	if err != nil {
		return nil, err
	}
	return Parse(data)
}

// Parse strictly decodes and validates a template document.
func Parse(data []byte) (*schema.Template, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var file templateFile
	if err := decoder.Decode(&file); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("template must contain exactly one YAML document")
	}
	if err := file.Envelope.Validate(); err != nil {
		return nil, err
	}
	if file.Envelope.Schema != schema.SchemaV1 {
		return nil, fmt.Errorf("unsupported schema %q", file.Envelope.Schema)
	}
	if file.Envelope.Kind != schema.KindTemplate {
		return nil, fmt.Errorf("expected kind %q, got %q", schema.KindTemplate, file.Envelope.Kind)
	}
	file.Template.Envelope = file.Envelope
	if err := validateSettings(&file.Template); err != nil {
		return nil, err
	}
	return &file.Template, nil
}

func validateSettings(template *schema.Template) error {
	if template.Page.Width <= 0 || template.Page.Height <= 0 {
		return fmt.Errorf("page width and height must be greater than zero")
	}
	if template.Margins.Top < 0 || template.Margins.Bottom < 0 || template.Margins.Left < 0 || template.Margins.Right < 0 {
		return fmt.Errorf("margins cannot be negative")
	}
	if strings.TrimSpace(template.Fonts.Regular) == "" || strings.TrimSpace(template.Fonts.Bold) == "" {
		return fmt.Errorf("regular and bold fonts are required")
	}
	for _, color := range []struct{ name, value string }{
		{"text", template.Colors.Text}, {"heading", template.Colors.Heading},
		{"link", template.Colors.Link}, {"rule", template.Colors.Rule}, {"background", template.Colors.Background},
	} {
		if color.value != "" && !validColor(color.value) {
			return fmt.Errorf("invalid %s color %q", color.name, color.value)
		}
	}
	for _, size := range []struct {
		name  string
		value float64
	}{
		{"name", template.Sizes.Name}, {"contact", template.Sizes.Contact},
		{"heading", template.Sizes.Heading}, {"roleTitle", template.Sizes.RoleTitle},
		{"body", template.Sizes.Body}, {"footer", template.Sizes.Footer},
	} {
		if size.value < 0 {
			return fmt.Errorf("%s font size cannot be negative", size.name)
		}
	}
	for _, setting := range []struct {
		name  string
		value float64
	}{
		{"line spacing", template.Spacing.Line}, {"section spacing", template.Spacing.Section},
		{"entry spacing", template.Spacing.EntryGap}, {"impact spacing", template.Spacing.ImpactGap},
		{"role spacing", template.Spacing.RoleGap},
		{"bullet spacing", template.Spacing.BulletGap},
		{"bullet indent", template.Indent.Bullet}, {"role indent", template.Indent.Role},
		{"accomplishment indent", template.Indent.Accomp},
	} {
		if setting.value < 0 {
			return fmt.Errorf("%s cannot be negative", setting.name)
		}
	}
	if template.Headings.RuleSize < 0 {
		return fmt.Errorf("heading rule size cannot be negative")
	}
	if template.PageBreaks.PageLimit < 0 {
		return fmt.Errorf("page limit cannot be negative")
	}
	if align := strings.ToLower(template.Header.Align); align != "" && align != "left" && align != "center" && align != "right" {
		return fmt.Errorf("unsupported header alignment %q", template.Header.Align)
	}
	if template.Headings.LetterSp != "" {
		if _, err := strconv.ParseFloat(template.Headings.LetterSp, 64); err != nil {
			return fmt.Errorf("invalid heading letter spacing %q", template.Headings.LetterSp)
		}
	}
	return nil
}

func validColor(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) == 3 {
		value = string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
	}
	if len(value) != 6 {
		return false
	}
	_, err := strconv.ParseUint(value, 16, 24)
	return err == nil
}
