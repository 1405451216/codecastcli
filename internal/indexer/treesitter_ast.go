//go:build cgo

package indexer

import (
	"fmt"
	"strings"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// extractTagsAST uses tree-sitter AST parsing to extract code tags.
// Returns nil if the language is not supported or parsing fails, so the caller
// can fall back to regex-based extraction.
func extractTagsAST(path string, content []byte, language string) []Tag {
	lang := getLanguage(language)
	if lang == nil {
		return nil
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(lang); err != nil {
		return nil
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
		return nil
	}
	defer tree.Close()

	root := tree.RootNode()

	switch language {
	case "go":
		return extractGoTagsAST(root, content)
	case "python":
		return extractPythonTagsAST(root, content)
	case "javascript":
		return extractJSTagsAST(root, content)
	case "typescript":
		return extractTSTagsAST(root, content)
	default:
		return nil
	}
}

// getLanguage returns the tree-sitter Language for the given language name.
func getLanguage(language string) *tree_sitter.Language {
	switch language {
	case "go":
		return tree_sitter.NewLanguage(tree_sitter_go.Language())
	case "python":
		return tree_sitter.NewLanguage(tree_sitter_python.Language())
	case "javascript":
		return tree_sitter.NewLanguage(tree_sitter_javascript.Language())
	case "typescript":
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
	default:
		return nil
	}
}

// ---------------------------------------------------------------------------
// Go AST tag extraction
// ---------------------------------------------------------------------------

// Go tree-sitter queries for extracting declarations.
// We walk the tree manually rather than using queries to avoid query
// compatibility issues across grammar versions.
func extractGoTagsAST(root *tree_sitter.Node, content []byte) []Tag {
	var tags []Tag
	cursor := root.Walk()
	defer cursor.Close()

	tags = walkGoNode(cursor, content, tags)
	return tags
}

func walkGoNode(cursor *tree_sitter.TreeCursor, content []byte, tags []Tag) []Tag {
	for {
		node := cursor.Node()
		kind := node.Kind()

		switch kind {
		case "function_declaration":
			tags = append(tags, goFuncTag(node, content, "function", ""))
		case "method_declaration":
			recv := goReceiverType(node, content)
			tags = append(tags, goFuncTag(node, content, "method", recv))
		case "interface_type":
			parent := node.Parent()
			if parent != nil && parent.Kind() == "type_declaration" {
				name := goTypeDeclName(parent, content)
				if name != "" {
					tags = append(tags, Tag{
						Name:      name,
						Kind:      "interface",
						Line:      int(parent.StartPosition().Row) + 1,
						Signature: fmt.Sprintf("type %s interface", name),
					})
				}
			}
		case "struct_type":
			parent := node.Parent()
			if parent != nil && parent.Kind() == "type_declaration" {
				name := goTypeDeclName(parent, content)
				if name != "" {
					tags = append(tags, Tag{
						Name:      name,
						Kind:      "struct",
						Line:      int(parent.StartPosition().Row) + 1,
						Signature: fmt.Sprintf("type %s struct", name),
					})
				}
			}
		case "type_alias":
			name := goTypeDeclName(node, content)
			if name != "" {
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "variable",
					Line:      int(node.StartPosition().Row) + 1,
					Signature: fmt.Sprintf("type %s", name),
				})
			}
		case "var_spec":
			tags = goVarTags(node, content, tags)
		}

		// Recurse into children
		if cursor.GoToFirstChild() {
			tags = walkGoNode(cursor, content, tags)
			cursor.GoToParent()
		}

		if !cursor.GoToNextSibling() {
			break
		}
	}
	return tags
}

func goFuncTag(node *tree_sitter.Node, content []byte, kind, receiver string) Tag {
	name := ""
	sig := ""

	// Find the name child
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		name = nameNode.Utf8Text(content)
	}

	// Build signature
	paramsNode := node.ChildByFieldName("parameters")
	resultNode := node.ChildByFieldName("result")

	if receiver != "" {
		recvNode := node.ChildByFieldName("receiver")
		recvText := ""
		if recvNode != nil {
			recvText = recvNode.Utf8Text(content)
		}
		sig = fmt.Sprintf("func %s %s", recvText, name)
	} else {
		sig = fmt.Sprintf("func %s", name)
	}

	if paramsNode != nil {
		sig += paramsNode.Utf8Text(content)
	}
	if resultNode != nil {
		sig += " " + resultNode.Utf8Text(content)
	}

	return Tag{
		Name:      name,
		Kind:      kind,
		Line:      int(node.StartPosition().Row) + 1,
		Signature: sig,
		Receiver:  receiver,
	}
}

func goReceiverType(node *tree_sitter.Node, content []byte) string {
	recvNode := node.ChildByFieldName("receiver")
	if recvNode == nil {
		return ""
	}
	// The receiver is like "(r *Type)" — we want the type name.
	// Walk children to find the type identifier.
	for i := uint(0); i < recvNode.ChildCount(); i++ {
		child := recvNode.Child(i)
		switch child.Kind() {
		case "pointer_type":
			// *Type — get the named child
			for j := uint(0); j < child.NamedChildCount(); j++ {
				nc := child.NamedChild(j)
				if nc.Kind() == "type_identifier" {
					return nc.Utf8Text(content)
				}
			}
		case "type_identifier":
			return child.Utf8Text(content)
		}
	}
	return ""
}

func goTypeDeclName(node *tree_sitter.Node, content []byte) string {
	// type_declaration has a name field in newer grammars, or we find the type_identifier child
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		return nameNode.Utf8Text(content)
	}
	// Fallback: look for type_identifier child
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "type_identifier" {
			return child.Utf8Text(content)
		}
	}
	return ""
}

func goVarTags(node *tree_sitter.Node, content []byte, tags []Tag) []Tag {
	// var_spec has one or more names
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() == "identifier" {
			name := child.Utf8Text(content)
			// Only exported vars
			if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "variable",
					Line:      int(node.StartPosition().Row) + 1,
					Signature: fmt.Sprintf("var %s", name),
				})
			}
		}
	}
	return tags
}

// ---------------------------------------------------------------------------
// Python AST tag extraction
// ---------------------------------------------------------------------------

func extractPythonTagsAST(root *tree_sitter.Node, content []byte) []Tag {
	var tags []Tag
	cursor := root.Walk()
	defer cursor.Close()

	tags = walkPythonNode(cursor, content, "", tags)
	return tags
}

func walkPythonNode(cursor *tree_sitter.TreeCursor, content []byte, currentClass string, tags []Tag) []Tag {
	for {
		node := cursor.Node()
		kind := node.Kind()

		switch kind {
		case "class_definition":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			sig := fmt.Sprintf("class %s", name)

			// Check for superclasses
			argListNode := node.ChildByFieldName("arguments")
			if argListNode != nil {
				args := strings.TrimSpace(argListNode.Utf8Text(content))
				if args != "" {
					sig = fmt.Sprintf("class %s(%s)", name, args)
				}
			}

			tags = append(tags, Tag{
				Name:      name,
				Kind:      "class",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: sig,
			})
			// Recurse with this class as the current class
			if cursor.GoToFirstChild() {
				tags = walkPythonNode(cursor, content, name, tags)
				cursor.GoToParent()
			}

		case "function_definition":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			paramsNode := node.ChildByFieldName("parameters")
			params := ""
			if paramsNode != nil {
				params = paramsNode.Utf8Text(content)
			}

			if currentClass != "" {
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "method",
					Line:      int(node.StartPosition().Row) + 1,
					Signature: fmt.Sprintf("def %s%s", name, params),
					Receiver:  currentClass,
				})
			} else {
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "function",
					Line:      int(node.StartPosition().Row) + 1,
					Signature: fmt.Sprintf("def %s%s", name, params),
				})
			}
			// Recurse into function body (for nested classes/functions)
			if cursor.GoToFirstChild() {
				tags = walkPythonNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}

		default:
			if cursor.GoToFirstChild() {
				tags = walkPythonNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}
		}

		if !cursor.GoToNextSibling() {
			break
		}
	}
	return tags
}

// ---------------------------------------------------------------------------
// JavaScript AST tag extraction
// ---------------------------------------------------------------------------

func extractJSTagsAST(root *tree_sitter.Node, content []byte) []Tag {
	var tags []Tag
	cursor := root.Walk()
	defer cursor.Close()

	tags = walkJSNode(cursor, content, "", tags)
	return tags
}

func walkJSNode(cursor *tree_sitter.TreeCursor, content []byte, currentClass string, tags []Tag) []Tag {
	for {
		node := cursor.Node()
		kind := node.Kind()

		switch kind {
		case "class_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "class",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("class %s", name),
			})
			// Recurse into class body for methods
			if cursor.GoToFirstChild() {
				tags = walkJSNode(cursor, content, name, tags)
				cursor.GoToParent()
			}

		case "function_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			paramsNode := node.ChildByFieldName("parameters")
			params := ""
			if paramsNode != nil {
				params = paramsNode.Utf8Text(content)
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("function %s%s", name, params),
			})
			if cursor.GoToFirstChild() {
				tags = walkJSNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}

		case "method_definition":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			if name == "constructor" {
				break
			}
			paramsNode := node.ChildByFieldName("parameters")
			params := ""
			if paramsNode != nil {
				params = paramsNode.Utf8Text(content)
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "method",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("%s%s", name, params),
				Receiver:  currentClass,
			})

		case "variable_declarator":
			// Arrow functions: const name = (params) => { ... }
			nameNode := node.ChildByFieldName("name")
			valueNode := node.ChildByFieldName("value")
			if nameNode != nil && valueNode != nil {
				valueKind := valueNode.Kind()
				if valueKind == "arrow_function" || valueKind == "function_expression" {
					name := nameNode.Utf8Text(content)
					paramsNode := valueNode.ChildByFieldName("parameters")
					params := ""
					if paramsNode != nil {
						params = paramsNode.Utf8Text(content)
					}
					tags = append(tags, Tag{
						Name:      name,
						Kind:      "function",
						Line:      int(node.StartPosition().Row) + 1,
						Signature: fmt.Sprintf("%s%s =>", name, params),
					})
				}
			}

		default:
			if cursor.GoToFirstChild() {
				tags = walkJSNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}
		}

		if !cursor.GoToNextSibling() {
			break
		}
	}
	return tags
}

// ---------------------------------------------------------------------------
// TypeScript AST tag extraction (extends JS with interfaces and type aliases)
// ---------------------------------------------------------------------------

func extractTSTagsAST(root *tree_sitter.Node, content []byte) []Tag {
	var tags []Tag
	cursor := root.Walk()
	defer cursor.Close()

	tags = walkTSNode(cursor, content, "", tags)
	return tags
}

func walkTSNode(cursor *tree_sitter.TreeCursor, content []byte, currentClass string, tags []Tag) []Tag {
	for {
		node := cursor.Node()
		kind := node.Kind()

		switch kind {
		case "class_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "class",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("class %s", name),
			})
			if cursor.GoToFirstChild() {
				tags = walkTSNode(cursor, content, name, tags)
				cursor.GoToParent()
			}

		case "interface_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "interface",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("interface %s", name),
			})

		case "type_alias_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "variable",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("type %s =", name),
			})

		case "function_declaration":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			paramsNode := node.ChildByFieldName("parameters")
			params := ""
			if paramsNode != nil {
				params = paramsNode.Utf8Text(content)
			}
			tags = append(tags, Tag{
				Name:      name,
				Kind:      "function",
				Line:      int(node.StartPosition().Row) + 1,
				Signature: fmt.Sprintf("function %s%s", name, params),
			})
			if cursor.GoToFirstChild() {
				tags = walkTSNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}

		case "method_definition", "public_field_definition":
			nameNode := node.ChildByFieldName("name")
			if nameNode == nil {
				break
			}
			name := nameNode.Utf8Text(content)
			if name == "constructor" {
				break
			}
			paramsNode := node.ChildByFieldName("parameters")
			params := ""
			if paramsNode != nil {
				params = paramsNode.Utf8Text(content)
			}
			// Only add as method if it has parameters (is callable)
			if params != "" || node.ChildByFieldName("body") != nil {
				tags = append(tags, Tag{
					Name:      name,
					Kind:      "method",
					Line:      int(node.StartPosition().Row) + 1,
					Signature: fmt.Sprintf("%s%s", name, params),
					Receiver:  currentClass,
				})
			}

		case "variable_declarator":
			nameNode := node.ChildByFieldName("name")
			valueNode := node.ChildByFieldName("value")
			if nameNode != nil && valueNode != nil {
				valueKind := valueNode.Kind()
				if valueKind == "arrow_function" || valueKind == "function_expression" {
					name := nameNode.Utf8Text(content)
					paramsNode := valueNode.ChildByFieldName("parameters")
					params := ""
					if paramsNode != nil {
						params = paramsNode.Utf8Text(content)
					}
					tags = append(tags, Tag{
						Name:      name,
						Kind:      "function",
						Line:      int(node.StartPosition().Row) + 1,
						Signature: fmt.Sprintf("%s%s =>", name, params),
					})
				}
			}

		default:
			if cursor.GoToFirstChild() {
				tags = walkTSNode(cursor, content, currentClass, tags)
				cursor.GoToParent()
			}
		}

		if !cursor.GoToNextSibling() {
			break
		}
	}
	return tags
}
