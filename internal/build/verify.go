package build

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/jlpdeveloper/resume-builder/internal/schema"
	pdfreader "github.com/ledongthuc/pdf"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// IntegrityFinding describes one visible-text integrity failure.
type IntegrityFinding struct {
	Code    string `json:"code"`
	Page    int    `json:"page,omitempty"`
	Text    string `json:"text,omitempty"`
	Message string `json:"message"`
}

// IntegrityError reports all detected visible-text violations.
type IntegrityError struct {
	Findings []IntegrityFinding
}

func (e *IntegrityError) Error() string {
	if len(e.Findings) == 0 {
		return "visible-text integrity check failed"
	}
	return fmt.Sprintf("visible-text integrity check failed: %s", e.Findings[0].Message)
}

var (
	zeroWidthChars = regexp.MustCompile(`[\x{00AD}\x{034F}\x{061C}\x{115F}\x{1160}\x{17B4}\x{17B5}\x{180B}-\x{180F}\x{200B}-\x{200F}\x{202A}-\x{202E}\x{2060}-\x{206F}\x{FEFF}\x{FFA0}]`)
	claimPatterns  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b\d+\+?\s+years?\s+(?:of\s+)?(?:professional\s+)?experience\b`),
		regexp.MustCompile(`(?i)\b(?:ph\.?d\.?|m\.?s\.?|b\.?s\.?|m\.?b\.?a\.?|certified|certification|security clearance|top secret|award[- ]winning|patent(?:ed)?|expert)\b`),
	}
	promptPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:ignore|disregard|forget|override)\s+(?:all\s+)?(?:previous|prior|above|earlier)\s+(?:instructions?|prompts?|rules?)\b`),
		regexp.MustCompile(`(?i)\b(?:system|developer|assistant)\s+prompt\b`),
		regexp.MustCompile(`(?i)\bwhen\s+you\s+are\s+(?:an?\s+)?(?:ai|language model|llm)\b`),
		regexp.MustCompile(`(?i)\b(?:keywords?|instructions?)\s+(?:for|only for)\s+(?:ai|ats|automated systems|bots?)\b`),
		regexp.MustCompile(`(?i)\bdo not\s+(?:tell|show|reveal)\b`),
	}
)

type textState struct {
	fontSize    float64
	textMode    int
	fillWhite   bool
	strokeWhite bool
}

// VerifyVisibleText compares extracted PDF text with text emitted by the renderer.
func VerifyVisibleText(pdfBytes []byte, expected []string, metadata schema.PDFMeta, background string) error {
	reader, err := pdfreader.NewReader(bytes.NewReader(pdfBytes), int64(len(pdfBytes)))
	if err != nil {
		return fmt.Errorf("read generated PDF text: %w", err)
	}
	info, err := api.PDFInfo(context.Background(), bytes.NewReader(pdfBytes), "resume.pdf", nil, false, model.NewDefaultConfiguration())
	if err != nil {
		return fmt.Errorf("inspect generated PDF: %w", err)
	}
	if reader.NumPage() != info.PageCount {
		return fmt.Errorf("PDF page count mismatch: text reader found %d pages, metadata reports %d", reader.NumPage(), info.PageCount)
	}

	findings := make([]IntegrityFinding, 0)
	actualText := make([]string, 0)
	for pageNumber := 1; pageNumber <= reader.NumPage(); pageNumber++ {
		page := reader.Page(pageNumber)
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			return fmt.Errorf("extract generated PDF text from page %d: %w", pageNumber, err)
		}
		actualText = append(actualText, pageText)
		content := page.Content()
		var bounds *model.PageBoundaries
		if pageNumber <= len(info.PageBoundaries) {
			bounds = &info.PageBoundaries[pageNumber-1]
		}
		for _, text := range content.Text {
			findings = append(findings, inspectText(text, bounds, pageNumber)...)
		}
	}
	findings = append(findings, compareText(expected, actualText)...)
	findings = append(findings, inspectPromptText(actualText)...)
	findings = append(findings, inspectMetadata(info, metadata, actualText)...)
	if backgroundIsWhite(background) {
		operatorFindings, err := inspectTextOperators(pdfBytes, true)
		if err != nil {
			return fmt.Errorf("inspect PDF text rendering operators: %w", err)
		}
		findings = append(findings, operatorFindings...)
	}
	if len(findings) > 0 {
		return &IntegrityError{Findings: findings}
	}
	return nil
}

func inspectText(text pdfreader.Text, bounds *model.PageBoundaries, page int) []IntegrityFinding {
	findings := make([]IntegrityFinding, 0)
	if text.FontSize <= 0 || math.IsNaN(text.FontSize) {
		findings = append(findings, IntegrityFinding{Code: "zero_size_text", Page: page, Text: text.S, Message: "PDF contains text with zero or invalid font size"})
	}
	if zeroWidthChars.MatchString(text.S) {
		findings = append(findings, IntegrityFinding{Code: "zero_width_text", Page: page, Text: text.S, Message: "PDF text contains zero-width characters"})
	}
	if bounds != nil {
		rect := bounds.MediaBox()
		if rect != nil {
			const tolerance = 2.0
			if text.X < rect.LL.X-tolerance || text.Y < rect.LL.Y-tolerance || text.X+text.W > rect.UR.X+tolerance || text.Y+text.FontSize > rect.UR.Y+tolerance {
				message := fmt.Sprintf("PDF text bounds x=%.1f y=%.1f w=%.1f size=%.1f exceed page (%.1f,%.1f)-(%.1f,%.1f)", text.X, text.Y, text.W, text.FontSize, rect.LL.X, rect.LL.Y, rect.UR.X, rect.UR.Y)
				findings = append(findings, IntegrityFinding{Code: "off_page_text", Page: page, Text: text.S, Message: message})
			}
		}
	}
	return findings
}

func compareText(expected, actual []string) []IntegrityFinding {
	expectedCounts := tokenCounts(expected)
	actualCounts := tokenCounts(actual)
	findings := make([]IntegrityFinding, 0)
	for token, count := range actualCounts {
		if count > expectedCounts[token] {
			findings = append(findings, IntegrityFinding{Code: "unexpected_text", Text: token, Message: fmt.Sprintf("PDF contains unexpected extracted text %q", token)})
		}
	}
	for token, count := range expectedCounts {
		if count > actualCounts[token] {
			findings = append(findings, IntegrityFinding{Code: "missing_visible_text", Text: token, Message: fmt.Sprintf("resolved visible text %q is missing from the PDF", token)})
		}
	}
	return findings
}

func tokenCounts(values []string) map[string]int {
	counts := make(map[string]int)
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		for _, token := range strings.Fields(value) {
			token = strings.TrimFunc(token, func(r rune) bool { return unicode.IsPunct(r) || unicode.IsSymbol(r) })
			if token == "" {
				continue
			}
			counts[token]++
		}
	}
	return counts
}

func inspectPromptText(values []string) []IntegrityFinding {
	findings := make([]IntegrityFinding, 0)
	for _, value := range values {
		for _, pattern := range promptPatterns {
			if match := pattern.FindString(value); match != "" {
				findings = append(findings, IntegrityFinding{Code: "prompt_injection", Text: match, Message: "PDF contains text that appears to instruct automated systems"})
			}
		}
	}
	return findings
}

func inspectMetadata(info *pdfcpu.PDFInfo, expected schema.PDFMeta, visible []string) []IntegrityFinding {
	findings := make([]IntegrityFinding, 0)
	for _, pair := range []struct{ name, actual, expected string }{
		{"title", info.Title, expected.Title},
		{"author", info.Author, expected.Author},
		{"subject", info.Subject, expected.Subject},
	} {
		if pair.actual != pair.expected {
			findings = append(findings, IntegrityFinding{Code: "unexpected_metadata", Text: pair.name, Message: fmt.Sprintf("PDF %s metadata does not match the declared metadata", pair.name)})
		}
		for _, pattern := range claimPatterns {
			for _, claim := range pattern.FindAllString(pair.actual, -1) {
				if !containsVisibleText(claim, visible) {
					findings = append(findings, IntegrityFinding{Code: "metadata_only_claim", Text: claim, Message: fmt.Sprintf("PDF %s metadata contains a claim absent from visible résumé text", pair.name)})
				}
			}
		}
	}
	for _, keyword := range info.Keywords {
		findings = append(findings, IntegrityFinding{Code: "hidden_metadata", Text: keyword, Message: "PDF contains keyword metadata outside ordinary document properties"})
	}
	for property, value := range info.Properties {
		findings = append(findings, IntegrityFinding{Code: "hidden_metadata", Text: property, Message: fmt.Sprintf("PDF contains custom metadata property %q", property)})
		for _, pattern := range promptPatterns {
			if match := pattern.FindString(value); match != "" {
				findings = append(findings, IntegrityFinding{Code: "prompt_injection", Text: match, Message: "PDF metadata contains text that appears to instruct automated systems"})
			}
		}
	}
	return findings
}

func containsVisibleText(value string, visible []string) bool {
	value = normalizeText(value)
	if value == "" {
		return true
	}
	for _, text := range visible {
		if strings.Contains(normalizeText(text), value) {
			return true
		}
	}
	return false
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func backgroundIsWhite(value string) bool {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	return value == "" || strings.EqualFold(value, "ffffff") || strings.EqualFold(value, "fff")
}

func inspectTextOperators(pdfBytes []byte, whiteBackground bool) ([]IntegrityFinding, error) {
	findings := make([]IntegrityFinding, 0)
	err := api.ExtractContent(context.Background(), bytes.NewReader(pdfBytes), nil, func(stream io.Reader, page int) error {
		data, err := io.ReadAll(stream)
		if err != nil {
			return err
		}
		findings = append(findings, scanTextOperators(data, page, whiteBackground)...)
		return nil
	}, model.NewDefaultConfiguration())
	return findings, err
}

func scanTextOperators(content []byte, page int, whiteBackground bool) []IntegrityFinding {
	findings := make([]IntegrityFinding, 0)
	state := textState{}
	stack := make([]textState, 0)
	operands := make([]string, 0)
	for _, token := range pdfTokens(content) {
		if !pdfOperators[token] {
			operands = append(operands, token)
			continue
		}
		switch token {
		case "q":
			stack = append(stack, state)
		case "Q":
			if len(stack) > 0 {
				state = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case "Tf":
			if len(operands) > 0 {
				state.fontSize, _ = strconv.ParseFloat(operands[len(operands)-1], 64)
				if state.fontSize <= 0 {
					findings = append(findings, IntegrityFinding{Code: "zero_size_text", Page: page, Message: "PDF text operator sets a zero or negative font size"})
				}
			}
		case "Tr":
			if len(operands) > 0 {
				state.textMode, _ = strconv.Atoi(operands[len(operands)-1])
				if state.textMode == 3 || state.textMode == 7 {
					findings = append(findings, IntegrityFinding{Code: "invisible_text", Page: page, Message: "PDF contains an invisible text rendering mode"})
				}
			}
		case "g":
			state.fillWhite = allNumbersNear(operands, 1, 1)
		case "rg":
			state.fillWhite = allNumbersNear(operands, 3, 1)
		case "k":
			state.fillWhite = allNumbersNear(operands, 4, 0)
		case "G":
			state.strokeWhite = allNumbersNear(operands, 1, 1)
		case "RG":
			state.strokeWhite = allNumbersNear(operands, 3, 1)
		case "K":
			state.strokeWhite = allNumbersNear(operands, 4, 0)
		case "Tj", "TJ", "'", "\"":
			if whiteBackground && ((state.textMode == 0 && state.fillWhite) || (state.textMode == 1 && state.strokeWhite) || (state.textMode == 2 && state.fillWhite && state.strokeWhite)) {
				findings = append(findings, IntegrityFinding{Code: "white_on_white_text", Page: page, Message: "PDF contains white text on a white page"})
			}
		}
		operands = operands[:0]
	}
	return findings
}

func allNumbersNear(values []string, count int, want float64) bool {
	if len(values) < count {
		return false
	}
	for _, value := range values[len(values)-count:] {
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.Abs(number-want) > 0.0001 {
			return false
		}
	}
	return true
}

var pdfOperators = map[string]bool{
	"q": true, "Q": true, "cm": true, "w": true, "J": true, "j": true, "M": true, "d": true,
	"ri": true, "i": true, "gs": true, "m": true, "l": true, "c": true, "v": true, "y": true,
	"h": true, "re": true, "S": true, "s": true, "f": true, "F": true, "f*": true, "B": true,
	"B*": true, "b": true, "b*": true, "n": true, "W": true, "W*": true, "BT": true, "ET": true,
	"Tc": true, "Tw": true, "Tz": true, "TL": true, "Tf": true, "Tr": true, "Ts": true, "Td": true,
	"TD": true, "Tm": true, "T*": true, "Tj": true, "TJ": true, "'": true, "\"": true,
	"d0": true, "d1": true, "CS": true, "cs": true, "SC": true, "SCN": true, "sc": true, "scn": true,
	"G": true, "g": true, "RG": true, "rg": true, "K": true, "k": true, "BX": true, "EX": true,
	"BMC": true, "BDC": true, "EMC": true, "sh": true, "Do": true,
}

func pdfTokens(data []byte) []string {
	tokens := make([]string, 0)
	for i := 0; i < len(data); {
		if isPDFSpace(data[i]) {
			i++
			continue
		}
		if data[i] == '%' {
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			continue
		}
		start := i
		switch data[i] {
		case '(', '<':
			if data[i] == '<' && i+1 < len(data) && data[i+1] == '<' {
				tokens = append(tokens, "<<")
				i += 2
				continue
			}
			delimiter := data[i]
			i++
			depth := 1
			for i < len(data) && depth > 0 {
				if data[i] == '\\' {
					i += 2
					continue
				}
				if delimiter == '(' && data[i] == '(' {
					depth++
				} else if (delimiter == '(' && data[i] == ')') || (delimiter == '<' && data[i] == '>') {
					depth--
				}
				i++
			}
			tokens = append(tokens, string(data[start:i]))
		case '[', ']', ')', '{', '}', '>':
			if data[i] == '>' && i+1 < len(data) && data[i+1] == '>' {
				tokens = append(tokens, ">>")
				i += 2
			} else {
				tokens = append(tokens, string(data[i]))
				i++
			}
		case '/':
			i++
			for i < len(data) && !isPDFSpace(data[i]) && !isPDFDelimiter(data[i]) {
				i++
			}
			tokens = append(tokens, string(data[start:i]))
		default:
			for i < len(data) && !isPDFSpace(data[i]) && !isPDFDelimiter(data[i]) {
				i++
			}
			tokens = append(tokens, string(data[start:i]))
		}
	}
	return tokens
}

func isPDFSpace(value byte) bool {
	return value == 0 || value == 9 || value == 10 || value == 12 || value == 13 || value == 32
}

func isPDFDelimiter(value byte) bool {
	switch value {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}
