package build

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	resumeTemplate "github.com/jlpdeveloper/resume-builder/internal/template"
	"github.com/johnfercher/go-tree/node"
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
	"github.com/phpdave11/gofpdf"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"
)

const defaultFontFamily = "ResumeSans"

// gofpdf writes font catalogs in map order unless sorting is enabled.
func init() {
	gofpdf.SetDefaultCatalogSort(true)
}

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
	expectedText                                  []string
	repeatedFooterName                            string
}

type itemBlock struct {
	first []core.Component
	rest  [][]core.Component
}

type contactItem struct {
	label string
	href  string
}

type verticalStack struct {
	components []core.Component
}

type verticalSpacer struct {
	height float64
}

func (*verticalSpacer) GetStructure() *node.Node[core.Structure] {
	return node.New(core.Structure{Type: "vertical_spacer"})
}

func (*verticalSpacer) SetConfig(*entity.Config) {}

func (s *verticalSpacer) GetHeight(_ core.Provider, _ *entity.Cell) float64 {
	return s.height
}

func (*verticalSpacer) Render(_ core.Provider, _ *entity.Cell) {}

func (s *verticalStack) GetStructure() *node.Node[core.Structure] {
	structure := node.New(core.Structure{Type: "vertical_stack"})
	for _, component := range s.components {
		structure.AddNext(component.GetStructure())
	}
	return structure
}

func (s *verticalStack) SetConfig(config *entity.Config) {
	for _, component := range s.components {
		component.SetConfig(config)
	}
}

func (s *verticalStack) GetHeight(provider core.Provider, cell *entity.Cell) float64 {
	height := 0.0
	for _, component := range s.components {
		height += component.GetHeight(provider, cell)
	}
	return height
}

func (s *verticalStack) Render(provider core.Provider, cell *entity.Cell) {
	current := cell.Copy()
	for _, component := range s.components {
		height := component.GetHeight(provider, &current)
		componentCell := current.Copy()
		componentCell.Height = height
		component.Render(provider, &componentCell)
		current.Y += height
	}
}

// Render generates a selectable-text PDF using the supplied template settings.
func Render(resolved *resolve.Resolved, template *schema.Template) (RenderResult, error) {
	return renderAt(resolved, template, time.Now().UTC())
}

func renderAt(resolved *resolve.Resolved, template *schema.Template, generatedAt time.Time) (RenderResult, error) {
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
		WithDefaultFont(&props.Font{Family: r.regular, Style: fontstyle.Normal, Size: r.bodySize, Color: r.textColor}).
		WithCreationDate(generatedAt)
	if footerPageEnabled(r.template.Footer.Page) {
		builder = builder.WithPageNumber(props.PageNumber{
			Pattern: "Page {current} of {total}", Place: props.RightBottom,
			Family: "Helvetica", Style: fontstyle.Normal, Size: float64(int(r.footerSize)), Color: r.textColor,
		})
	}
	if r.template.Page.Width > 0 && r.template.Page.Height > 0 {
		builder = builder.WithDimensions(r.template.Page.Width, r.template.Page.Height)
	} else {
		builder = builder.WithPageSize(pagesize.Letter)
	}
	bottomMargin := r.template.Margins.Bottom
	if footerPageEnabled(r.template.Footer.Page) {
		minimumFooterMargin := r.footerSize * 4 * 25.4 / 72
		if bottomMargin < minimumFooterMargin {
			bottomMargin = minimumFooterMargin
		}
	}
	builder = builder.
		WithTopMargin(r.template.Margins.Top).
		WithRightMargin(r.template.Margins.Right).
		WithBottomMargin(bottomMargin).
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
	m := maroto.New(builder.Build())
	if footerNameEnabled(r.template.Footer.Name) {
		r.repeatedFooterName = resolved.Header.Name
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
		headingRow := []core.Component{r.textWithColor(heading, r.bold, fontstyle.Bold, r.headingSize, align.Left, 0, r.template.Spacing.Section, 1, r.headingColor)}
		if boolValue(r.template.Headings.Rule, false) {
			headingRow = append(headingRow, line.New(props.Line{
				Color: r.ruleColor, Style: linestyle.Solid, Thickness: positive(r.template.Headings.RuleSize, 0.2),
				Orientation: orientation.Horizontal, SizePercent: 100,
			}))
			headingRow = append(headingRow, &verticalSpacer{height: 2})
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
			entryGap := r.template.Spacing.EntryGap
			if section == "impact" {
				entryGap = r.template.Spacing.ImpactGap
			}
			r.addEntryGap(m, entryGap)
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
	if err := setPDFModificationDate(pdfBytes, generatedAt); err != nil {
		return RenderResult{}, fmt.Errorf("set PDF modification date: %w", err)
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
	expectedText := append([]string{}, r.expectedText...)
	for i := 1; i < pageCount; i++ {
		if r.repeatedFooterName != "" {
			expectedText = append(expectedText, r.repeatedFooterName)
		}
	}
	if footerPageEnabled(r.template.Footer.Page) {
		for page := 1; page <= pageCount; page++ {
			expectedText = append(expectedText, fmt.Sprintf("Page %d of %d", page, pageCount))
		}
	}
	if err := VerifyVisibleText(pdfBytes, expectedText, resolved.Metadata, r.template.Colors.Background); err != nil {
		return RenderResult{}, fmt.Errorf("visible-text integrity check failed: %w", err)
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

func setPDFModificationDate(pdfBytes []byte, timestamp time.Time) error {
	index := bytes.LastIndex(pdfBytes, []byte("/ModDate"))
	if index < 0 {
		return fmt.Errorf("PDF Info dictionary has no ModDate")
	}
	position := index + len("/ModDate")
	for position < len(pdfBytes) && isPDFWhitespace(pdfBytes[position]) {
		position++
	}
	if position >= len(pdfBytes) || pdfBytes[position] != '(' {
		return fmt.Errorf("PDF Info dictionary has invalid ModDate")
	}
	start := position + 1
	end := bytes.IndexByte(pdfBytes[start:], ')')
	if end < 0 {
		return fmt.Errorf("PDF Info dictionary has unterminated ModDate")
	}
	end += start
	date := timestamp.UTC().Format("D:20060102150405")
	if end-start == len(date)+len("+00'00'") {
		date += "+00'00'"
	}
	if end-start != len(date) {
		return fmt.Errorf("PDF ModDate has unexpected string length %d", end-start)
	}
	copy(pdfBytes[start:end], date)
	return nil
}

func isPDFWhitespace(value byte) bool {
	switch value {
	case 0, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func (r *renderer) renderHeader(m core.Maroto, resolved *resolve.Resolved) {
	black := &props.Color{Red: 0, Green: 0, Blue: 0}
	m.AddAutoRow(col.New(12).Add(r.textWithColor(resolved.Header.Name, r.bold, fontstyle.Bold, r.nameSize, align.Center, 0, 0, 0, black)))
	addHeaderSpacer(m, 1)

	contact := make([]contactItem, 0, 6+len(resolved.Header.Portfolio))
	addContact := func(label, href string) {
		if strings.TrimSpace(label) != "" {
			contact = append(contact, contactItem{label: label, href: href})
		}
	}
	addContact(resolved.Header.Phone, "")
	addContact(resolved.Header.Email, "mailto:"+resolved.Header.Email)
	addContact(resolved.Header.Location, "")
	if resolved.Header.GitHub != "" {
		addContact("GitHub", resolved.Header.GitHub)
	}
	if resolved.Header.LinkedIn != "" {
		addContact("LinkedIn", resolved.Header.LinkedIn)
	}
	for _, credential := range resolved.Header.Credentials {
		addContact(credential, "")
	}
	for _, link := range resolved.Header.Portfolio {
		addContact(link.Label, link.URL)
	}
	itemsPerRow := 3
	if !r.contactCompact {
		itemsPerRow = 1
	}
	for start := 0; start < len(contact); start += itemsPerRow {
		if start > 0 {
			addHeaderSpacer(m, 0.8)
		}
		end := min(start+itemsPerRow, len(contact))
		m.AddAutoRow(r.contactRow(contact[start:end], black)...)
	}

	positioning := false
	if resolved.Headline != "" {
		addHeaderSpacer(m, 2)
		m.AddAutoRow(col.New(12).Add(r.text(resolved.Headline, r.regular, fontstyle.Normal, r.bodySize, align.Center, 0, 0)))
		positioning = true
	}
	targetText := joinNonEmpty(" — ", resolved.Target.Company, resolved.Target.Role)
	if targetText != "" {
		if positioning {
			addHeaderSpacer(m, 0.8)
		} else {
			addHeaderSpacer(m, 2)
		}
		var target core.Component
		if resolved.Target.URL != "" {
			target = r.linkAligned(targetText, resolved.Target.URL, r.bodySize, align.Center)
		} else {
			target = r.text(targetText, r.regular, fontstyle.Normal, r.bodySize, align.Center, 0, 0)
		}
		m.AddAutoRow(col.New(12).Add(target))
		positioning = true
	}
	if resolved.Target.Notes != "" {
		if positioning {
			addHeaderSpacer(m, 0.8)
		} else {
			addHeaderSpacer(m, 2)
		}
		m.AddAutoRow(col.New(12).Add(r.text(resolved.Target.Notes, r.regular, fontstyle.Normal, r.bodySize, align.Center, 0, 0)))
		positioning = true
	}

	addHeaderSpacer(m, 3.5)
	m.AddAutoRow(col.New(12).Add(line.New(props.Line{
		Color: &props.Color{Red: 160, Green: 160, Blue: 160}, Style: linestyle.Solid,
		Thickness: 0.25, Orientation: orientation.Horizontal, SizePercent: 100,
	})))
	addHeaderSpacer(m, 3)
}

func (r *renderer) contactRow(items []contactItem, separatorColor *props.Color) []core.Col {
	if len(items) == 1 {
		return []core.Col{col.New(12).Add(r.contactComponent(items[0], align.Center))}
	}

	sizes := contactSegmentSizes(items)
	columns := make([]core.Col, 0, len(sizes)+2)
	columns = append(columns, col.New(1))
	segment := 0
	for i, item := range items {
		if i > 0 {
			columns = append(columns, col.New(sizes[segment]).Add(r.textWithColor(" | ", r.regular, fontstyle.Normal, r.contactSize, align.Center, 0, 0, 0, separatorColor)))
			segment++
		}
		columns = append(columns, col.New(sizes[segment]).Add(r.contactComponent(item, align.Center)))
		segment++
	}
	columns = append(columns, col.New(1))
	return columns
}

func (r *renderer) contactComponent(item contactItem, textAlign align.Type) core.Component {
	if item.href != "" {
		return r.linkAligned(item.label, item.href, r.contactSize, textAlign)
	}
	return r.text(item.label, r.regular, fontstyle.Normal, r.contactSize, textAlign, 0, 0)
}

func addHeaderSpacer(m core.Maroto, height float64) {
	m.AddRows(row.New(height).Add(col.New(12)))
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
				r.textWithColor(group.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0, 1, r.textColor),
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
			var name core.Component
			if cert.CredentialURL != "" {
				name = r.link(cert.Name, cert.CredentialURL, r.bodySize)
			} else {
				name = r.text(cert.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
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
	var company core.Component
	if employer.Link != "" {
		company = r.link(name, employer.Link, r.bodySize)
	} else {
		company = r.text(name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
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
	var name core.Component
	if project.Link != "" {
		name = r.link(project.Name, project.Link, r.bodySize)
	} else {
		name = r.text(project.Name, r.bold, fontstyle.Bold, r.bodySize, align.Left, 0, 0)
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
	return r.textWithColor(value, family, style, size, textAlign, left, top, 0, r.textColor)
}

func (r *renderer) textWithColor(value, family string, style fontstyle.Type, size float64, textAlign align.Type, left, top, bottom float64, color *props.Color) core.Component {
	if strings.TrimSpace(value) != "" {
		r.expectedText = append(r.expectedText, value)
	}
	return text.New(value, props.Text{
		Family: family, Style: style, Size: size, Align: textAlign,
		Color: color, Left: left, Top: top, Bottom: bottom, VerticalPadding: r.template.Spacing.Line,
	})
}

func (r *renderer) link(label, target string, size float64) core.Component {
	return r.linkAligned(label, target, size, align.Left)
}

func (r *renderer) linkAligned(label, target string, size float64, textAlign align.Type) core.Component {
	url := target
	if strings.TrimSpace(label) != "" {
		r.expectedText = append(r.expectedText, label)
	}
	return text.New(label, props.Text{
		Family: r.regular, Style: fontstyle.Normal, Size: size,
		Color: r.linkColor, Align: textAlign, Hyperlink: &url,
	})
}

func (r *renderer) addRow(m core.Maroto, components []core.Component) {
	if len(components) > 0 {
		m.AddAutoRow(col.New(12).Add(&verticalStack{components: components}))
	}
}

func (r *renderer) addEntryGap(m core.Maroto, gap float64) {
	if gap > 0 {
		m.AddRows(row.New(gap).Add(col.New(12)))
	}
}

func contactSegmentSizes(items []contactItem) []int {
	segmentCount := len(items)*2 - 1
	if segmentCount <= 0 {
		return nil
	}
	sizes := make([]int, segmentCount)
	weights := make([]int, segmentCount)
	totalWeight := 0
	for i, item := range items {
		segment := i * 2
		weights[segment] = len([]rune(item.label))
		totalWeight += weights[segment]
		if i > 0 {
			weights[segment-1] = 3
			totalWeight += weights[segment-1]
		}
	}
	if totalWeight == 0 {
		return sizes
	}
	remaining := 10 - segmentCount
	remainders := make([]int, segmentCount)
	assigned := 0
	for i, weight := range weights {
		sizes[i] = 1 + remaining*weight/totalWeight
		remainders[i] = remaining * weight % totalWeight
		assigned += sizes[i] - 1
	}
	for i := 0; i < remaining-assigned; i++ {
		best := 0
		for j := 1; j < len(remainders); j++ {
			if remainders[j] > remainders[best] {
				best = j
			}
		}
		sizes[best]++
		remainders[best] = -1
	}
	return sizes
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
