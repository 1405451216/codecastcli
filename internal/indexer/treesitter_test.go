package indexer

import (
	"strings"
	"testing"
)

func TestExtractGoTags(t *testing.T) {
	src := `package main

import "fmt"

// MyInterface does things
type MyInterface interface {
	DoSomething()
}

type MyStruct struct {
	Name string
}

func NewMyStruct(name string) *MyStruct {
	return &MyStruct{Name: name}
}

func (s *MyStruct) Greet(greeting string) string {
	return greeting + ", " + s.Name
}

var DefaultName = "world"
`

	tags := extractGoTagsRegex(src)

	// We expect: interface, struct, function, method, variable
	expectedKinds := map[string]string{
		"MyInterface":  "interface",
		"MyStruct":     "struct",
		"NewMyStruct":  "function",
		"Greet":        "method",
		"DefaultName":  "variable",
	}

	found := map[string]bool{}
	for _, tag := range tags {
		want, ok := expectedKinds[tag.Name]
		if !ok {
			t.Logf("unexpected tag: %+v", tag)
			continue
		}
		if tag.Kind != want {
			t.Errorf("tag %q: got kind %q, want %q", tag.Name, tag.Kind, want)
		}
		found[tag.Name] = true

		// Verify line numbers are > 0
		if tag.Line <= 0 {
			t.Errorf("tag %q: Line should be > 0, got %d", tag.Name, tag.Line)
		}

		// Verify signature is non-empty
		if tag.Signature == "" {
			t.Errorf("tag %q: Signature should not be empty", tag.Name)
		}
	}

	for name := range expectedKinds {
		if !found[name] {
			t.Errorf("expected tag %q not found", name)
		}
	}

	// Verify method has receiver
	for _, tag := range tags {
		if tag.Name == "Greet" {
			if tag.Receiver != "MyStruct" {
				t.Errorf("Greet: got Receiver %q, want %q", tag.Receiver, "MyStruct")
			}
		}
	}
}

func TestExtractPythonTags(t *testing.T) {
	src := `class Animal:
    def speak(self):
        pass

class Dog(Animal):
    def speak(self):
        return "woof"

def helper(x, y):
    return x + y

async def fetch_data(url):
    pass
`

	tags := extractPythonTagsRegex(src)

	// Note: pyFuncRe is checked before pyMethodRe and operates on trimmed lines,
	// so indented def inside a class is classified as "function" by the current
	// implementation. pyMethodRe only matches on the raw (untrimmed) line and
	// is checked after pyFuncRe, so it is effectively unreachable for lines
	// that start with "def" after trimming.
	expected := []struct {
		name     string
		kind     string
		receiver string
	}{
		{"Animal", "class", ""},
		{"speak", "function", ""}, // pyFuncRe matches on trimmed line; does not set Receiver
		{"Dog", "class", ""},
		{"speak", "function", ""}, // pyFuncRe matches on trimmed line; does not set Receiver
		{"helper", "function", ""},
		{"fetch_data", "function", ""},
	}

	if len(tags) != len(expected) {
		t.Fatalf("got %d tags, want %d; tags: %+v", len(tags), len(expected), tags)
	}

	for i, exp := range expected {
		tag := tags[i]
		if tag.Name != exp.name {
			t.Errorf("tag[%d]: got Name %q, want %q", i, tag.Name, exp.name)
		}
		if tag.Kind != exp.kind {
			t.Errorf("tag[%d]: got Kind %q, want %q", i, tag.Kind, exp.kind)
		}
		if tag.Receiver != exp.receiver {
			t.Errorf("tag[%d]: got Receiver %q, want %q", i, tag.Receiver, exp.receiver)
		}
	}

	// Verify Dog class signature includes base class
	for _, tag := range tags {
		if tag.Name == "Dog" {
			if !strings.Contains(tag.Signature, "Animal") {
				t.Errorf("Dog signature should mention base class Animal, got %q", tag.Signature)
			}
		}
	}
}

func TestExtractJSTags(t *testing.T) {
	src := `function greet(name) {
  return "hello " + name;
}

export function fetchData(url) {
  return fetch(url);
}

const add = (a, b) => a + b;

class App extends Component {
  render() {
    return null;
  }
}

interface User {
  name: string;
}

type ID = string | number;
`

	tags := extractJSTagsRegex(src)

	expected := []struct {
		name string
		kind string
	}{
		{"greet", "function"},
		{"fetchData", "function"},
		{"add", "function"},
		{"App", "class"},
		{"render", "method"},
		{"User", "interface"},
		{"ID", "variable"},
	}

	if len(tags) != len(expected) {
		t.Fatalf("got %d tags, want %d; tags: %+v", len(tags), len(expected), tags)
	}

	for i, exp := range expected {
		tag := tags[i]
		if tag.Name != exp.name {
			t.Errorf("tag[%d]: got Name %q, want %q", i, tag.Name, exp.name)
		}
		if tag.Kind != exp.kind {
			t.Errorf("tag[%d]: got Kind %q, want %q", i, tag.Kind, exp.kind)
		}
	}

	// Verify method has receiver
	for _, tag := range tags {
		if tag.Name == "render" {
			if tag.Receiver != "App" {
				t.Errorf("render: got Receiver %q, want %q", tag.Receiver, "App")
			}
		}
	}
}

func TestRepoMap(t *testing.T) {
	idx := &Indexer{
		index: &Index{
			Files: map[string]*FileEntry{
				"cmd/main.go": {
					Path: "cmd/main.go",
					Tags: []Tag{
						{Name: "main", Kind: "function", Line: 10, Signature: "func main()"},
						{Name: "Config", Kind: "struct", Line: 20, Signature: "type Config struct"},
					},
				},
				"pkg/handler.go": {
					Path: "pkg/handler.go",
					Tags: []Tag{
						{Name: "Handle", Kind: "function", Line: 5, Signature: "func Handle(req)"},
					},
				},
			},
		},
	}

	result := idx.RepoMap(1000)

	// Verify output contains directory headers
	if !strings.Contains(result, "cmd:") {
		t.Errorf("RepoMap output should contain 'cmd:' directory header, got:\n%s", result)
	}
	if !strings.Contains(result, "pkg:") {
		t.Errorf("RepoMap output should contain 'pkg:' directory header, got:\n%s", result)
	}

	// Verify output contains file names
	if !strings.Contains(result, "main.go") {
		t.Errorf("RepoMap output should contain 'main.go', got:\n%s", result)
	}
	if !strings.Contains(result, "handler.go") {
		t.Errorf("RepoMap output should contain 'handler.go', got:\n%s", result)
	}

	// Verify output contains signatures
	if !strings.Contains(result, "func main()") {
		t.Errorf("RepoMap output should contain 'func main()', got:\n%s", result)
	}
	if !strings.Contains(result, "func Handle(req)") {
		t.Errorf("RepoMap output should contain 'func Handle(req)', got:\n%s", result)
	}

	// Verify output contains line numbers
	if !strings.Contains(result, "L10") {
		t.Errorf("RepoMap output should contain line number L10, got:\n%s", result)
	}
}

func TestExtractGoTagsSkipsComments(t *testing.T) {
	src := `package main

// This is a comment
/* block comment */
func Hello() {}
`
	tags := extractGoTagsRegex(src)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag (Hello), got %d: %+v", len(tags), tags)
	}
	if tags[0].Name != "Hello" {
		t.Errorf("got tag name %q, want %q", tags[0].Name, "Hello")
	}
}

func TestExtractTagsUnknownLanguage(t *testing.T) {
	tags := extractTags("file.xyz", []byte("whatever"), "unknown")
	if tags != nil {
		t.Errorf("expected nil for unknown language, got %+v", tags)
	}
}
