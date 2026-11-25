package dependencies

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jlpdeveloper/resume-builder/internal/resolve"
)

// Node is one file in a resolved dependency graph.
type Node struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Reference describes a file-to-file dependency.
type Reference struct {
	From  string `json:"from"`
	Field string `json:"field"`
	To    string `json:"to"`
}

// Graph lists every resolved file and the references between them.
type Graph struct {
	Root       string      `json:"root"`
	Nodes      []Node      `json:"nodes"`
	References []Reference `json:"references"`
}

type fileReference struct {
	field string
	path  string
}

// Resolve builds the dependency graph for a YAML file.
func Resolve(path string) (Graph, error) {
	resolved, err := (&resolve.Resolver{}).Resolve(path)
	if err != nil {
		return Graph{}, err
	}

	graph := Graph{
		Root:       resolved.Root,
		Nodes:      make([]Node, 0, len(resolved.Documents)),
		References: []Reference{},
	}
	paths := make(map[string]bool, len(resolved.Documents))
	for _, doc := range resolved.Documents {
		paths[doc.Path] = true
		graph.Nodes = append(graph.Nodes, Node{Path: doc.Path, Kind: string(doc.Kind), ID: doc.Envelope.ID})
	}
	for _, doc := range resolved.Documents {
		for _, ref := range fileReferences(doc) {
			to, err := filepath.Abs(filepath.Join(filepath.Dir(doc.Path), ref.path))
			if err != nil {
				return Graph{}, fmt.Errorf("resolve %s reference from %q: %w", ref.field, doc.Path, err)
			}
			if !paths[to] {
				return Graph{}, fmt.Errorf("%s reference from %q is not in the resolved graph: %q", ref.field, doc.Path, to)
			}
			graph.References = append(graph.References, Reference{From: doc.Path, Field: ref.field, To: to})
		}
	}
	sort.Slice(graph.Nodes, func(i, j int) bool {
		if graph.Nodes[i].Path == graph.Nodes[j].Path {
			return false
		}
		if graph.Nodes[i].Path == graph.Root {
			return true
		}
		if graph.Nodes[j].Path == graph.Root {
			return false
		}
		return graph.Nodes[i].Path < graph.Nodes[j].Path
	})
	sort.Slice(graph.References, func(i, j int) bool {
		if graph.References[i].From != graph.References[j].From {
			return graph.References[i].From < graph.References[j].From
		}
		return graph.References[i].Field < graph.References[j].Field
	})
	return graph, nil
}

// Run implements the dependencies command.
func Run(args []string, stdout, stderr io.Writer, jsonOutput bool) int {
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		fmt.Fprintln(stderr, "Usage: resume dependencies PATH")
		return 2
	}
	graph, err := Resolve(args[0])
	if err != nil {
		if jsonOutput {
			_ = json.NewEncoder(stdout).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	if jsonOutput {
		if err := json.NewEncoder(stdout).Encode(graph); err != nil {
			fmt.Fprintf(stderr, "write dependencies output: %v\n", err)
			return 1
		}
	} else {
		writeTree(stdout, graph)
	}
	return 0
}

func fileReferences(doc *resolve.Document) []fileReference {
	refs := []fileReference{
		{field: "profile", path: doc.Profile},
		{field: "base", path: doc.Base},
		{field: "template", path: doc.Template},
	}
	result := refs[:0]
	for _, ref := range refs {
		if ref.path != "" {
			result = append(result, ref)
		}
	}
	return result
}

func writeTree(out io.Writer, graph Graph) {
	nodes := make(map[string]Node, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.Path] = node
	}
	fmt.Fprintf(out, "%s\n", nodeLabel(nodes[graph.Root], graph.Root, graph.Root))
	visited := map[string]bool{graph.Root: true}
	writeReferences(out, graph, nodes, graph.Root, "", visited)
}

func writeReferences(out io.Writer, graph Graph, nodes map[string]Node, from, prefix string, visited map[string]bool) {
	refs := make([]Reference, 0)
	for _, ref := range graph.References {
		if ref.From == from {
			refs = append(refs, ref)
		}
	}
	for i, ref := range refs {
		last := i == len(refs)-1
		branch := "├──"
		nextPrefix := prefix + "│   "
		if last {
			branch = "└──"
			nextPrefix = prefix + "    "
		}
		fmt.Fprintf(out, "%s%s %s: %s", prefix, branch, ref.Field, nodeLabel(nodes[ref.To], ref.To, graph.Root))
		if visited[ref.To] {
			fmt.Fprintln(out, " (already shown)")
			continue
		}
		fmt.Fprintln(out)
		visited[ref.To] = true
		writeReferences(out, graph, nodes, ref.To, nextPrefix, visited)
	}
}

func nodeLabel(node Node, path, root string) string {
	return fmt.Sprintf("%s [%s, id=%s]", displayPath(root, path), node.Kind, node.ID)
}

func displayPath(root, path string) string {
	relative, err := filepath.Rel(filepath.Dir(root), path)
	if err != nil {
		return path
	}
	return relative
}
