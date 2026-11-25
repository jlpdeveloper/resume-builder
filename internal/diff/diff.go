package diff

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
	"github.com/jlpdeveloper/resume-builder/internal/schema"
	"github.com/jlpdeveloper/resume-builder/internal/validate"
)

// Change describes one difference between two resolved resumes.
type Change struct {
	Type    string `json:"type"`
	Section string `json:"section,omitempty"`
	Kind    string `json:"kind,omitempty"`
	ID      string `json:"id,omitempty"`
	Field   string `json:"field,omitempty"`
	Before  any    `json:"before"`
	After   any    `json:"after"`
}

// Result contains the differences found between two resumes.
type Result struct {
	Identical bool     `json:"identical"`
	Changes   []Change `json:"changes"`
}

type resolvedResume struct {
	content *resolve.Resolved
	ids     map[schema.Kind]string
}

type item struct {
	key     string
	section string
	kind    string
	id      string
	field   string
	value   any
}

type snapshot struct {
	items  []item
	orders map[string][]string
}

// Compare validates and resolves both resumes before comparing their contents.
func Compare(left, right string) (Result, error) {
	leftResume, err := load(left)
	if err != nil {
		return Result{}, fmt.Errorf("left resume: %w", err)
	}
	rightResume, err := load(right)
	if err != nil {
		return Result{}, fmt.Errorf("right resume: %w", err)
	}

	result := Result{Changes: []Change{}}
	compareMetadata(&result, leftResume, rightResume)
	compareContent(&result, snapshotContent(leftResume.content), snapshotContent(rightResume.content))
	result.Identical = len(result.Changes) == 0
	return result, nil
}

// Run implements the diff command.
func Run(args []string, stdout, stderr io.Writer, jsonOutput bool) int {
	if len(args) != 2 || strings.TrimSpace(args[0]) == "" || strings.TrimSpace(args[1]) == "" {
		fmt.Fprintln(stderr, "Usage: resume diff LEFT RIGHT")
		return 2
	}
	result, err := Compare(args[0], args[1])
	if err != nil {
		if jsonOutput {
			_ = json.NewEncoder(stdout).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(result); err != nil {
			fmt.Fprintf(stderr, "write diff output: %v\n", err)
			return 1
		}
	} else {
		writeResult(stdout, result)
	}
	if result.Identical {
		return 0
	}
	return 1
}

func load(path string) (*resolvedResume, error) {
	validation := validate.Validate(path)
	if !validation.Valid {
		if len(validation.Errors) == 0 {
			return nil, fmt.Errorf("validation failed")
		}
		first := validation.Errors[0]
		return nil, fmt.Errorf("%s:%d: %s", first.Path, first.Line, first.Message)
	}
	graph, err := (&resolve.Resolver{}).Resolve(path)
	if err != nil {
		return nil, err
	}
	content, err := resolve.ResolveContent(graph)
	if err != nil {
		return nil, err
	}
	ids := make(map[schema.Kind]string)
	for _, doc := range graph.Documents {
		ids[doc.Kind] = doc.Envelope.ID
	}
	return &resolvedResume{content: content, ids: ids}, nil
}

func compareMetadata(result *Result, left, right *resolvedResume) {
	addChanged(result, "metadata", "resume", left.ids[schema.KindResume], "id", left.ids[schema.KindResume], right.ids[schema.KindResume])
	addChanged(result, "metadata", "profile", left.ids[schema.KindProfile], "id", left.ids[schema.KindProfile], right.ids[schema.KindProfile])
	addChanged(result, "metadata", "base", left.ids[schema.KindBase], "id", left.ids[schema.KindBase], right.ids[schema.KindBase])
	addChanged(result, "metadata", "template", left.ids[schema.KindTemplate], "id", left.ids[schema.KindTemplate], right.ids[schema.KindTemplate])
	addChanged(result, "metadata", "target", "", "company", left.content.Target.Company, right.content.Target.Company)
	addChanged(result, "metadata", "target", "", "role", left.content.Target.Role, right.content.Target.Role)
	addChanged(result, "metadata", "target", "", "url", left.content.Target.URL, right.content.Target.URL)
	addChanged(result, "metadata", "target", "", "notes", left.content.Target.Notes, right.content.Target.Notes)
	addChanged(result, "metadata", "resume", "", "headline", left.content.Headline, right.content.Headline)
	addChanged(result, "metadata", "resume", "", "pageLimit", left.content.PageLimit, right.content.PageLimit)
	addChanged(result, "metadata", "resume", "", "metadata", left.content.Metadata, right.content.Metadata)
	if !reflect.DeepEqual(left.content.Template, right.content.Template) {
		result.Changes = append(result.Changes, Change{
			Type: "changed", Section: "metadata", Kind: "template", Field: "settings",
			Before: jsonString(left.content.Template), After: jsonString(right.content.Template),
		})
	}
	leftSections := left.content.Sections
	rightSections := right.content.Sections
	if !reflect.DeepEqual(leftSections, rightSections) {
		changeType := "changed"
		if sameItems(leftSections, rightSections) {
			changeType = "reordered"
		}
		result.Changes = append(result.Changes, Change{
			Type: changeType, Section: "sections", Field: "order", Before: leftSections, After: rightSections,
		})
	}
}

func snapshotContent(resolved *resolve.Resolved) snapshot {
	s := snapshot{orders: make(map[string][]string)}
	if resolved.Summary != nil {
		id := resolved.Summary.ID
		if id == "" {
			id = "inline"
		}
		s.add("summary", "summary", id, "text", resolved.Summary.Text)
	}
	for i, impact := range resolved.Impact {
		id := impact.ID
		if id == "" {
			id = fmt.Sprint(i)
		}
		s.addOrder("impact", id)
		s.add("impact", "impact", id, "text", impact.Text)
	}
	for _, group := range resolved.Skills {
		s.addOrder("skills", group.ID)
		s.add("skills", "skill-group", group.ID, "name", group.Name)
		for _, skill := range group.Skills {
			s.addOrder("skills/"+group.ID+"/items", skill)
			s.add("skills", "skill", group.ID+"/"+skill, "name", skill)
		}
	}
	for _, employer := range resolved.Experience {
		s.addOrder("experience", employer.ID)
		kind := "employer"
		s.add("experience", kind, employer.ID, "name", employer.Name)
		s.add("experience", kind, employer.ID, "location", employer.Location)
		s.add("experience", kind, employer.ID, "link", employer.Link)
		roleOrder := "experience/" + employer.ID + "/roles"
		for _, role := range employer.Roles {
			s.addOrder(roleOrder, role.ID)
			roleKind := "role"
			roleKey := employer.ID + "/" + role.ID
			s.add("experience", roleKind, roleKey, "title", role.Title)
			s.add("experience", roleKind, roleKey, "start", role.Start)
			s.add("experience", roleKind, roleKey, "end", role.End)
			bulletOrder := "experience/" + roleKey + "/accomplishments"
			for index, bullet := range role.Accomplishments {
				bulletID := fmt.Sprint(index)
				s.addOrder(bulletOrder, bullet)
				s.add("experience", "accomplishment", roleKey+"/"+bulletID, "text", bullet)
			}
		}
	}
	for _, project := range resolved.Projects {
		s.addOrder("projects", project.ID)
		for field, value := range map[string]any{
			"name": project.Name, "role": project.Role, "start": project.Start, "end": project.End,
			"link": project.Link, "tech": project.Tech,
		} {
			s.add("projects", "project", project.ID, field, value)
		}
		for index, bullet := range project.Accomplishments {
			bulletID := fmt.Sprint(index)
			s.addOrder("projects/"+project.ID+"/accomplishments", bullet)
			s.add("projects", "project-accomplishment", project.ID+"/"+bulletID, "text", bullet)
		}
	}
	for _, education := range resolved.Education {
		s.addOrder("education", education.ID)
		for field, value := range map[string]any{
			"institution": education.Institution, "degree": education.Degree,
			"field": education.Field, "graduation": education.Graduation, "honors": education.Honors,
		} {
			s.add("education", "education", education.ID, field, value)
		}
	}
	for _, cert := range resolved.Certs {
		s.addOrder("certifications", cert.ID)
		for field, value := range map[string]any{
			"name": cert.Name, "issuer": cert.Issuer, "dateEarned": cert.DateEarned,
			"expiration": cert.Expiration, "credentialUrl": cert.CredentialURL,
		} {
			s.add("certifications", "certification", cert.ID, field, value)
		}
	}
	return s
}

func (s *snapshot) add(section, kind, id, field string, value any) {
	s.items = append(s.items, item{
		key:     section + "/" + kind + "/" + id + "/" + field,
		section: section, kind: kind, id: id, field: field, value: value,
	})
}

func (s *snapshot) addOrder(group, id string) {
	s.orders[group] = append(s.orders[group], id)
}

func compareContent(result *Result, left, right snapshot) {
	leftItems := make(map[string]item, len(left.items))
	rightItems := make(map[string]item, len(right.items))
	keys := make(map[string]bool, len(left.items)+len(right.items))
	for _, entry := range left.items {
		leftItems[entry.key] = entry
		keys[entry.key] = true
	}
	for _, entry := range right.items {
		rightItems[entry.key] = entry
		keys[entry.key] = true
	}
	sortedKeys := make([]string, 0, len(keys))
	for key := range keys {
		sortedKeys = append(sortedKeys, key)
	}
	sort.Strings(sortedKeys)
	for _, key := range sortedKeys {
		before, inLeft := leftItems[key]
		after, inRight := rightItems[key]
		switch {
		case !inLeft:
			result.Changes = append(result.Changes, Change{Type: "added", Section: after.section, Kind: after.kind, ID: after.id, Field: after.field, After: after.value})
		case !inRight:
			result.Changes = append(result.Changes, Change{Type: "removed", Section: before.section, Kind: before.kind, ID: before.id, Field: before.field, Before: before.value})
		case !reflect.DeepEqual(before.value, after.value):
			result.Changes = append(result.Changes, Change{Type: "changed", Section: before.section, Kind: before.kind, ID: before.id, Field: before.field, Before: before.value, After: after.value})
		}
	}

	groups := make(map[string]bool, len(left.orders)+len(right.orders))
	for group := range left.orders {
		groups[group] = true
	}
	for group := range right.orders {
		groups[group] = true
	}
	sortedGroups := make([]string, 0, len(groups))
	for group := range groups {
		sortedGroups = append(sortedGroups, group)
	}
	sort.Strings(sortedGroups)
	for _, group := range sortedGroups {
		before, after := left.orders[group], right.orders[group]
		if !reflect.DeepEqual(before, after) && sameItems(before, after) {
			result.Changes = append(result.Changes, Change{Type: "reordered", Section: group, Field: "order", Before: before, After: after})
		}
	}
}

func addChanged(result *Result, section, kind, id, field string, before, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	result.Changes = append(result.Changes, Change{
		Type: "changed", Section: section, Kind: kind, ID: id, Field: field, Before: before, After: after,
	})
}

func sameItems(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	counts := make(map[string]int, len(left))
	for _, item := range left {
		counts[item]++
	}
	for _, item := range right {
		counts[item]--
		if counts[item] < 0 {
			return false
		}
	}
	return true
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func writeResult(out io.Writer, result Result) {
	if result.Identical {
		fmt.Fprintln(out, "Resumes are identical.")
		return
	}
	fmt.Fprintln(out, "Resume differences:")
	for _, change := range result.Changes {
		label := change.Section
		if change.ID != "" {
			label += "/" + change.ID
		}
		if change.Field != "" {
			label += "/" + change.Field
		}
		fmt.Fprintf(out, "- %s %s", change.Type, label)
		if change.Before != nil {
			fmt.Fprintf(out, "\n    before: %v", change.Before)
		}
		if change.After != nil {
			fmt.Fprintf(out, "\n    after:  %v", change.After)
		}
		fmt.Fprintln(out)
	}
}
