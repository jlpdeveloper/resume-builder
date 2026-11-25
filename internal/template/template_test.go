package template

import (
	"testing"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
)

func TestStandardContainsPresentationSettingsOnly(t *testing.T) {
	standard, err := Standard()
	if err != nil {
		t.Fatal(err)
	}
	if standard.Envelope.Kind != schema.KindTemplate || standard.Envelope.ID != "standard" {
		t.Fatalf("unexpected template envelope: %+v", standard.Envelope)
	}
	if standard.Page.Width <= 0 || standard.Page.Height <= 0 || standard.Fonts.Regular == "" {
		t.Fatalf("missing page or font settings: %+v", standard)
	}
	if standard.Header.Align != "center" || standard.Header.Compact == nil || !*standard.Header.Compact {
		t.Fatalf("expected centered compact header: %+v", standard.Header)
	}
	if standard.Headings.Rule == nil || !*standard.Headings.Rule || standard.Colors.Heading != "#1F4E79" {
		t.Fatalf("expected blue ruled headings: %+v", standard.Headings)
	}
	if standard.Footer.Name == nil || !*standard.Footer.Name || standard.Footer.Page == nil || !*standard.Footer.Page {
		t.Fatalf("expected candidate name and page-number footer: %+v", standard.Footer)
	}
}

func TestParseRejectsResumeContent(t *testing.T) {
	data := []byte("schema: resume/v1\nkind: template\nid: standard\npage:\n  width: 216\n  height: 279\nemployment: []\n")
	if _, err := Parse(data); err == nil {
		t.Fatal("expected content fields to be rejected in a template")
	}
}

func TestParseRejectsWrongKindAndInvalidSettings(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
	}{
		{
			name: "wrong kind",
			data: "schema: resume/v1\nkind: profile\nid: standard\n",
		},
		{
			name: "bad color",
			data: "schema: resume/v1\nkind: template\nid: standard\npage:\n  width: 216\n  height: 279\nfonts:\n  regular: ResumeSans\n  bold: ResumeSans\ncolors:\n  heading: blue\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Parse([]byte(test.data)); err == nil {
				t.Fatal("expected template parse to fail")
			}
		})
	}
}
