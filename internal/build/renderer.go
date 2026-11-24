package build

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	resumeTemplate "github.com/jlpdeveloper/resume-builder/internal/template"
	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/linestyle"
	"github.com/johnfercher/maroto/v2/pkg/consts/orientation"
	"github.com/johnfercher/maroto/v2/pkg/consts/pagesize"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/core/entity"
	"github.com/johnfercher/maroto/v2/pkg/fontrepository"
	"github.com/johnfercher/maroto/v2/pkg/props"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
)

const defaultFontFamily = "ResumeSans"

// RenderResult contains the generated PDF and its page count.
type RenderResult struct {
	PDF       []byte
	PageCount int
	PageLimit int
	Warnings  []Warning
}

// Warning reports a non-fatal rendering condition.
type Warning struct {
	Code    string `json:"code"`
	Section string `json:"section,omitempty"`
	Field   string `json:"field,omitempty"`
	Path    string `json:"path,omitempty"`
	Line    int    `json:"line,omitempty"`
	Message string `json:"message"`
}

type renderer struct {
	template                                      schema.Template
	regular, bold, italic, boldItalic             string
	textColor, headingColor, linkColor, ruleColor *props.Color
	nameSize, contactSize, headingSize, roleSize  float64
	bodySize, footerSize                          float64
	headingAlign                                  align.Type
	contactCompact                                bool
}

type itemBlock struct {
	first []core.Component
	rest  [][]core.Component
}

// Render generates a selectable-text PDF using the supplied template settings.
func Render(resolved *resolve.Resolved, template *schema.Template) (RenderResult, error) {
	if resolved == nil {
		return RenderResult{}, fmt.Errorf("resolved resume is required")
	}
	if template == nil {
		if resolved.TemplatePath != "" && resolved.Template != nil {
			template = resolved.Template
		} else {
			var err error
			template, err = resumeTemplate.Standard()
			if err != nil {
				return RenderResult{}, fmt.Errorf("load standard template: %w", err)
			}
		}
	}
	r := newRenderer(template)

	fonts, err := embeddedFonts(r)
	if err != nil {
		return RenderResult{}, fmt.Errorf("load embedded fonts: %w", err)
	}
	builder := config.NewBuilder().
		WithCustomFonts(fonts).
		WithDefaultFont(&props.Font{Family: r.regular, Style: fontstyle.Normal, Size: r.bodySize, Color: r.textColor})
	if r.template.Page.Width > 0 && r.template.Page.Height > 0 {
		builder = builder.WithDimensions(r.template.Page.Width, r.template.Page.Height)
	} else {
		builder = builder.WithPageSize(pagesize.Letter)
	}
	builder = builder.
		WithTopMargin(r.template.Margins.Top).
		WithRightMargin(r.template.Margins.Right).
		WithBottomMargin(r.template.Margins.Bottom).
		WithLeftMargin(r.template.Margins.Left)
	if resolved.Metadata.Title != "" {
		builder = builder.WithTitle(resolved.Metadata.Title, true)
	}
	if resolved.Metadata.Author != "" {
		builder = builder.WithAuthor(resolved.Metadata.Author, true)
	}
	if resolved.Metadata.Subject != "" {
		builder = builder.WithSubject(resolved.Metadata.Subject, true)
	}
	if footerPageEnabled(r.template.Footer.Page) {
		builder = builder.WithPageNumber(props.PageNumber{
			Pattern: "Page {current} of {total}", Place: props.RightBottom,
			Family: r.regular, Style: fontstyle.Normal, Size: r.footerSize, Color: r.textColor,
		})
	}

	m := maroto.New(builder.Build())
	if footerNameEnabled(r.template.Footer.Name) {
		footer := row.New(8).Add(col.New(12).Add(r.text(resolved.Header.Name, r.regular, fontstyle.Normal, r.footerSize, align.Left, 0, 0)))
		if err := m.RegisterFooter(footer); err != nil {
			return RenderResult{}, fmt.Errorf("register PDF footer: %w", err)
		}
	}

	r.renderHeader(m, resolved)
	lastSection := ""
	for _, section := range resolved.Sections {
		blocks := r.sectionBlocks(section, resolved)
		if len(blocks) == 0 {
			continue
		}
		lastSection = section
		heading := sectionHeading(section, r.template.Headings.Uppercase)
		headingRow := []core.Component{r.textWithColor(heading, r.bold, fontstyle.Bold, r.headingSize, align.Left, 0, r.template.Spacing.Section, r.headingColor)}
		if boolValue(r.template.Headings.Rule, false) {
			headingRow = append(headingRow, line.New(props.Line{
				Color: r.ruleColor, Style: linestyle.Solid, Thickness: positive(r.template.Headings.RuleSize, 0.2),
				Orientation: orientation.Horizontal, SizePercent: 100,
			}))
		}
		if boolValue(r.template.PageBreaks.KeepHeadingWithContent, true) {
			headingRow = append(headingRow, blocks[0].first...)
			r.addRow(m, headingRow)
		} else {
			r.addRow(m, headingRow)
			r.addRow(m, blocks[0].first)
		}
		r.addRows(m, blocks[0].rest)
		for _, block := range blocks[1:] {
			r.addRow(m, block.first)
			r.addRows(m, block.rest)
		}
	}

	doc, err := m.Generate()
	if err != nil {
		return RenderResult{}, fmt.Errorf("generate PDF: %w", err)
	}
	pdfBytes := doc.GetBytes()
	pageCount, err := api.PageCount(context.Background(), bytes.NewReader(pdfBytes), model.NewDefaultConfiguration())
	if err != nil {
		return RenderResult{}, fmt.Errorf("count PDF pages: %w", err)
	}
	pageLimit := resolved.PageLimit
	if pageLimit <= 0 {
		pageLimit = r.template.PageBreaks.PageLimit
	}
	warnings := []Warning{}
	if pageLimit > 0 && pageCount > pageLimit {
		message := fmt.Sprintf("page limit of %d exceeded: generated %d pages", pageLimit, pageCount)
		if lastSection != "" {
			message += fmt.Sprintf("; overflow reached section %q", lastSection)
		}
		warnings = append(warnings, Warning{Code: "page_limit_exceeded", Section: lastSection, Message: message})
	}
	return RenderResult{PDF: pdfBytes, PageCount: pageCount, PageLimit: pageLimit, Warnings: warnings}, nil
}

func newRenderer(template *schema.Template) *renderer {
	r := &renderer{}
	if template != nil {
		r.template = *template
	} else {
		r.template.Margins = schema.MarginConfig{Top: 16, Bottom: 20, Left: 16, Right: 16}
	}
	fonts := r.template.Fonts
	r.regular = nonEmpty(fonts.Regular, defaultFontFamily)
	r.bold = nonEmpty(fonts.Bold, r.regular)
	r.italic = nonEmpty(fonts.Italic, r.regular)
	r.boldItalic = nonEmpty(fonts.BoldIt, r.bold)
	r.textColor = parseColor(r.template.Colors.Text, &props.Color{Red: 35, Green: 40, Blue: 48})
	r.headingColor = parseColor(r.template.Colors.Heading, &props.Color{Red: 31, Green: 78, Blue: 121})
	r.linkColor = parseColor(r.template.Colors.Link, &props.Color{Red: 31, Green: 78, Blue: 121})
	r.ruleColor = parseColor(r.template.Colors.Rule, r.headingColor)
	r.nameSize = positive(r.template.Sizes.Name, 20)
	r.contactSize = positive(r.template.Sizes.Contact, 8)
	r.headingSize = positive(r.template.Sizes.Heading, 12)
	r.roleSize = positive(r.template.Sizes.RoleTitle, 10)
	r.bodySize = positive(r.template.Sizes.Body, 9.5)
	r.footerSize = positive(r.template.Sizes.Footer, 8)
	r.contactCompact = boolValue(r.template.Header.Compact, true)
	r.headingAlign = align.Left
	switch strings.ToLower(r.template.Header.Align) {
	case "center":
		r.headingAlign = align.Center
	case "right":
		r.headingAlign = align.Right
	}
	return r
}

func embeddedFonts(r *renderer) ([]entity.CustomFont, error) {
	repository := fontrepository.New()
	repository.AddUTF8FontFromBytes(r.regular, fontstyle.Normal, goregular.TTF)
	repository.AddUTF8FontFromBytes(r.bold, fontstyle.Bold, gobold.TTF)
	repository.AddUTF8FontFromBytes(r.italic, fontstyle.Italic, goitalic.TTF)
	repository.AddUTF8FontFromBytes(r.boldItalic, fontstyle.BoldItalic, gobolditalic.TTF)
	fonts, err := repository.Load()
	if err != nil {
		return nil, err
	}
	return fonts, nil
}

func (r *renderer) renderHeader(m core.Maroto, resolved *resolve.Resolved) {
	m.AddAutoRow(col.New(12).Add(r.text(resolved.Header.Name, r.bold, fontstyle.Bold, r.nameSize, r.headingAlign, 0, 0)))

	contact := make([]core.Component, 0, 12)
	addContact := func(label, href string) {
		if r.contactCompact && len(contact) > 0 {
			contact = append(contact, r.text(" | ", r.regular, fontstyle.Normal, r.contactSize, align.Center, 0, 0))
		}
		if href != "" {
			contact = append(contact, r.link(label, href, r.contactSize))
		} else if label != "" {
			contact = append(contact, r.text(label, r.regular, fontstyle.Normal, r.contactSize, align.Center, 0, 0))
		}
	}
	addContact(resolved.Header.Phone, "")
	addContact(resolved.Header.Email, "mailto:"+resolved.Header.Email)
	addContact(resolved.Header.Location, "")
	addContact("GitHub", resolved.Header.GitHub)
	addContact("LinkedIn", resolved.Header.LinkedIn)
	for _, link := range resolved.Header.Portfolio {
		addContact(link.Label, link.URL)
	}
	if len(contact) > 0 {
		if r.contactCompact {
			m.AddAutoRow(col.New(12).Add(contact...))
		} else {
			for _, item := range contact {
				m.AddAutoRow(col.New(12).Add(item))
			}
		}
	}

	targetText := resolved.Target.Company + " — " + resolved.Target.Role
	if resolved.Target.URL != "" {
		m.AddAutoRow(col.New(12).Add(r.link(targetText, resolved.Target.URL, r.bodySize)))
	} else {
		m.AddAutoRow(col.New(12).Add(r.text(targetText, r.regular, fontstyle.Normal, r.bodySize, r.headingAlign, 0, 0)))
	}
	if resolved.Headline != "" {
		m.AddAutoRow(col.New(12).Add(r.text(resolved.Headline, r.regular, fontstyle.Normal, r.bodySize, r.headingAlign, 0, 0)))
	}
	if resolved.Target.Notes != "" {
		m.AddAutoRow(col.New(12).Add(r.text(resolved.Target.Notes, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0)))
	}
	m.AddRows(row.New(3).Add(col.New(12)))
}

func (r *renderer) sectionBlocks(section string, resolved *resolve.Resolved) []itemBlock {
	blocks := make([]itemBlock, 0)
	switch section {
	case "summary":
		if resolved.Summary != nil {
			blocks = append(blocks, itemBlock{first: []core.Component{r.text(resolved.Summary.Text, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0)}})
		}
	case "impact":
		for _, impact := range resolved.Impact {
			blocks = append(blocks, itemBlock{first: []core.Component{r.bullet(impact.Text, r.template.Indent.Bullet)}})
		}
	case "skills":
		for _, group := range resolved.Skills {
			block := itemBlock{first: []core.Component{
				r.text(group.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0),
				r.text(strings.Join(group.Skills, ", "), r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0),
			}}
			blocks = append(blocks, block)
		}
	case "experience":
		for _, employer := range resolved.Experience {
			blocks = append(blocks, r.experienceBlock(employer))
		}
	case "projects":
		for _, project := range resolved.Projects {
			blocks = append(blocks, r.projectBlock(project))
		}
	case "education":
		for _, education := range resolved.Education {
			block := itemBlock{first: []core.Component{
				r.text(education.Institution, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0),
				r.text(education.Degree, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0),
			}}
			if education.Field != "" || education.Graduation != "" || education.Honors != "" {
				block.rest = append(block.rest, []core.Component{r.text(joinNonEmpty(" · ", education.Field, education.Graduation, education.Honors), r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0)})
			}
			blocks = append(blocks, block)
		}
	case "certifications":
		for _, cert := range resolved.Certs {
			name := r.text(cert.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
			if cert.CredentialURL != "" {
				name = r.link(cert.Name, cert.CredentialURL, r.bodySize)
			}
			block := itemBlock{first: []core.Component{
				name,
				r.text(cert.Issuer, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0),
			}}
			if detail := joinNonEmpty(" · ", cert.DateEarned, cert.Expiration); detail != "" {
				block.rest = append(block.rest, []core.Component{r.text(detail, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0)})
			}
			blocks = append(blocks, block)
		}
	}
	return blocks
}

func (r *renderer) experienceBlock(employer resolve.ResolvedEmployer) itemBlock {
	name := employer.Name
	if employer.Location != "" {
		name += " — " + employer.Location
	}
	company := r.text(name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
	if employer.Link != "" {
		company = r.link(name, employer.Link, r.bodySize)
	}
	block := itemBlock{first: []core.Component{company}}
	keepRoleWithAccomplishment := boolValue(r.template.PageBreaks.KeepRoleWithAccomp, true)
	for roleIndex, role := range employer.Roles {
		roleTitle := role.Title
		if role.Start != "" {
			roleTitle += " · " + role.Start
		}
		if role.End != "" {
			roleTitle += "–" + role.End
		}
		roleComponent := r.text(roleTitle, r.bold, fontstyle.Bold, r.roleSize, align.Left, r.template.Indent.Role, r.template.Spacing.RoleGap)
		if roleIndex == 0 {
			block.first = append(block.first, roleComponent)
			if len(role.Accomplishments) > 0 {
				bullet := r.bullet(role.Accomplishments[0], r.template.Indent.Accomp)
				if keepRoleWithAccomplishment {
					block.first = append(block.first, bullet)
				} else {
					block.rest = append(block.rest, []core.Component{bullet})
				}
			}
		} else {
			first := []core.Component{roleComponent}
			if len(role.Accomplishments) > 0 && keepRoleWithAccomplishment {
				first = append(first, r.bullet(role.Accomplishments[0], r.template.Indent.Accomp))
			}
			block.rest = append(block.rest, first)
			if len(role.Accomplishments) > 0 && !keepRoleWithAccomplishment {
				block.rest = append(block.rest, []core.Component{r.bullet(role.Accomplishments[0], r.template.Indent.Accomp)})
			}
		}
		start := 0
		if len(role.Accomplishments) > 0 {
			start = 1
		}
		for _, accomplishment := range role.Accomplishments[start:] {
			block.rest = append(block.rest, []core.Component{r.bullet(accomplishment, r.template.Indent.Accomp)})
		}
	}
	return block
}

func (r *renderer) projectBlock(project schema.Project) itemBlock {
	name := r.text(project.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
	if project.Link != "" {
		name = r.link(project.Name, project.Link, r.bodySize)
	}
	block := itemBlock{first: []core.Component{name}}
	if project.Role != "" {
		block.first = append(block.first, r.text(project.Role, r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0))
	}
	if project.Tech != "" {
		block.rest = append(block.rest, []core.Component{r.text(project.Tech, r.italic, fontstyle.Italic, r.bodySize, align.Left, 0, 0)})
	}
	if project.Start != "" || project.End != "" {
		block.rest = append(block.rest, []core.Component{r.text(joinNonEmpty("–", project.Start, project.End), r.regular, fontstyle.Normal, r.bodySize, align.Left, 0, 0)})
	}
	for index, accomplishment := range project.Accomplishments {
		bullet := r.bullet(accomplishment, r.template.Indent.Accomp)
		if index == 0 {
			block.first = append(block.first, bullet)
		} else {
			block.rest = append(block.rest, []core.Component{bullet})
		}
	}
	return block
}

func (r *renderer) bullet(value string, indent float64) core.Component {
	return r.text("•  "+value, r.regular, fontstyle.Normal, r.bodySize, align.Left, indent, r.template.Spacing.BulletGap)
}

func (r *renderer) text(value, family string, style fontstyle.Type, size float64, textAlign align.Type, left, top float64) core.Component {
	return r.textWithColor(value, family, style, size, textAlign, left, top, r.textColor)
}

func (r *renderer) textWithColor(value, family string, style fontstyle.Type, size float64, textAlign align.Type, left, top float64, color *props.Color) core.Component {
	return text.New(value, props.Text{
		Family: family, Style: style, Size: size, Align: textAlign,
		Color: color, Left: left, Top: top, VerticalPadding: r.template.Spacing.Line,
	})
}

func (r *renderer) link(label, target string, size float64) core.Component {
	url := target
	return text.New(label, props.Text{
		Family: r.regular, Style: fontstyle.Normal, Size: size,
		Color: r.linkColor, Hyperlink: &url,
	})
}

func (r *renderer) addRow(m core.Maroto, components []core.Component) {
	if len(components) > 0 {
		m.AddAutoRow(col.New(12).Add(components...))
	}
}

func (r *renderer) addRows(m core.Maroto, rows [][]core.Component) {
	for _, components := range rows {
		r.addRow(m, components)
	}
}

func sectionHeading(section string, uppercase *bool) string {
	heading := strings.ToUpper(section[:1]) + section[1:]
	if uppercase != nil && *uppercase {
		heading = strings.ToUpper(heading)
	}
	return heading
}

func footerNameEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func footerPageEnabled(enabled *bool) bool {
	return enabled == nil || *enabled
}

func parseColor(value string, fallback *props.Color) *props.Color {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) == 3 {
		value = string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
	}
	if len(value) != 6 {
		return fallback
	}
	red, errRed := strconv.ParseUint(value[0:2], 16, 8)
	green, errGreen := strconv.ParseUint(value[2:4], 16, 8)
	blue, errBlue := strconv.ParseUint(value[4:6], 16, 8)
	if errRed != nil || errGreen != nil || errBlue != nil {
		return fallback
	}
	return &props.Color{Red: int(red), Green: int(green), Blue: int(blue)}
}

func positive(value, fallback float64) float64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func joinNonEmpty(separator string, values ...string) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return strings.Join(result, separator)
}
