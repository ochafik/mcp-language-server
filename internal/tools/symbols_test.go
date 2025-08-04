package tools

import (
	"regexp"
	"testing"

	"github.com/isaacphi/mcp-language-server/internal/protocol"
	"github.com/stretchr/testify/assert"
)

func TestParseSymbolKind(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected protocol.SymbolKind
	}{
		{"File", "file", protocol.File},
		{"File uppercase", "File", protocol.File},
		{"File mixed case", "FILE", protocol.File},
		{"Module", "module", protocol.Module},
		{"Namespace", "namespace", protocol.Namespace},
		{"Package", "package", protocol.Package},
		{"Class", "class", protocol.Class},
		{"Method", "method", protocol.Method},
		{"Property", "property", protocol.Property},
		{"Field", "field", protocol.Field},
		{"Constructor", "constructor", protocol.Constructor},
		{"Enum", "enum", protocol.Enum},
		{"Interface", "interface", protocol.Interface},
		{"Function", "function", protocol.Function},
		{"Variable", "variable", protocol.Variable},
		{"Constant", "constant", protocol.Constant},
		{"String", "string", protocol.String},
		{"Number", "number", protocol.Number},
		{"Boolean", "boolean", protocol.Boolean},
		{"Array", "array", protocol.Array},
		{"Object", "object", protocol.Object},
		{"Key", "key", protocol.Key},
		{"Null", "null", protocol.Null},
		{"EnumMember", "enummember", protocol.EnumMember},
		{"Struct", "struct", protocol.Struct},
		{"Event", "event", protocol.Event},
		{"Operator", "operator", protocol.Operator},
		{"TypeParameter", "typeparameter", protocol.TypeParameter},
		{"Unknown type", "unknown", 0},
		{"Empty string", "", 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := parseSymbolKind(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFlattenDocumentSymbols(t *testing.T) {
	symbols := []protocol.DocumentSymbol{
		{
			Name: "TestClass",
			Kind: protocol.Class,
			Range: protocol.Range{
				Start: protocol.Position{Line: 0, Character: 0},
				End:   protocol.Position{Line: 10, Character: 0},
			},
			Detail: "class TestClass",
			Children: []protocol.DocumentSymbol{
				{
					Name: "method1",
					Kind: protocol.Method,
					Range: protocol.Range{
						Start: protocol.Position{Line: 2, Character: 4},
						End:   protocol.Position{Line: 4, Character: 4},
					},
					Detail: "void method1()",
				},
				{
					Name: "field1",
					Kind: protocol.Field,
					Range: protocol.Range{
						Start: protocol.Position{Line: 1, Character: 4},
						End:   protocol.Position{Line: 1, Character: 15},
					},
					Detail: "int field1",
				},
			},
		},
		{
			Name: "function1",
			Kind: protocol.Function,
			Range: protocol.Range{
				Start: protocol.Position{Line: 12, Character: 0},
				End:   protocol.Position{Line: 15, Character: 0},
			},
			Detail: "void function1()",
		},
	}

	t.Run("No filter", func(t *testing.T) {
		results := flattenDocumentSymbols(symbols, "", map[protocol.SymbolKind]bool{})
		assert.Len(t, results, 4) // TestClass, method1, field1, function1
		
		assert.Equal(t, "TestClass", results[0].Name)
		assert.Equal(t, "Class", results[0].Kind)
		assert.Equal(t, "", results[0].Container)
		
		assert.Equal(t, "method1", results[1].Name)
		assert.Equal(t, "Method", results[1].Kind)
		assert.Equal(t, "TestClass", results[1].Container)
		
		assert.Equal(t, "field1", results[2].Name)
		assert.Equal(t, "Field", results[2].Kind)
		assert.Equal(t, "TestClass", results[2].Container)
		
		assert.Equal(t, "function1", results[3].Name)
		assert.Equal(t, "Function", results[3].Kind)
		assert.Equal(t, "", results[3].Container)
	})

	t.Run("Filter by Class only", func(t *testing.T) {
		filter := map[protocol.SymbolKind]bool{protocol.Class: true}
		results := flattenDocumentSymbols(symbols, "", filter)
		assert.Len(t, results, 1)
		assert.Equal(t, "TestClass", results[0].Name)
		assert.Equal(t, "Class", results[0].Kind)
	})

	t.Run("Filter by Method and Function", func(t *testing.T) {
		filter := map[protocol.SymbolKind]bool{
			protocol.Method:   true,
			protocol.Function: true,
		}
		results := flattenDocumentSymbols(symbols, "", filter)
		assert.Len(t, results, 2)
		
		assert.Equal(t, "method1", results[0].Name)
		assert.Equal(t, "Method", results[0].Kind)
		
		assert.Equal(t, "function1", results[1].Name)
		assert.Equal(t, "Function", results[1].Kind)
	})

	t.Run("Nested container", func(t *testing.T) {
		results := flattenDocumentSymbols(symbols, "OuterClass", map[protocol.SymbolKind]bool{})
		assert.Len(t, results, 4)
		
		assert.Equal(t, "OuterClass", results[0].Container)
		assert.Equal(t, "OuterClass.TestClass", results[1].Container)
		assert.Equal(t, "OuterClass.TestClass", results[2].Container)
		assert.Equal(t, "OuterClass", results[3].Container)
	})
}

func TestConvertSymbolInformation(t *testing.T) {
	symbols := []protocol.SymbolInformation{
		{
			Name: "testFunction",
			Kind: protocol.Function,
			Location: protocol.Location{
				URI: "file:///path/to/file.go",
				Range: protocol.Range{
					Start: protocol.Position{Line: 5, Character: 0},
					End:   protocol.Position{Line: 10, Character: 0},
				},
			},
			ContainerName: "main",
		},
		{
			Name: "TestStruct",
			Kind: protocol.Struct,
			Location: protocol.Location{
				URI: "file:///path/to/types.go",
				Range: protocol.Range{
					Start: protocol.Position{Line: 15, Character: 0},
					End:   protocol.Position{Line: 20, Character: 0},
				},
			},
			ContainerName: "",
		},
	}

	t.Run("No filter", func(t *testing.T) {
		results := convertSymbolInformation(symbols, map[protocol.SymbolKind]bool{})
		assert.Len(t, results, 2)
		
		assert.Equal(t, "testFunction", results[0].Name)
		assert.Equal(t, "Function", results[0].Kind)
		assert.Equal(t, "/path/to/file.go:6:1", results[0].Location)
		assert.Equal(t, "main", results[0].Container)
		
		assert.Equal(t, "TestStruct", results[1].Name)
		assert.Equal(t, "Struct", results[1].Kind)
		assert.Equal(t, "/path/to/types.go:16:1", results[1].Location)
		assert.Equal(t, "", results[1].Container)
	})

	t.Run("Filter by Function only", func(t *testing.T) {
		filter := map[protocol.SymbolKind]bool{protocol.Function: true}
		results := convertSymbolInformation(symbols, filter)
		assert.Len(t, results, 1)
		assert.Equal(t, "testFunction", results[0].Name)
		assert.Equal(t, "Function", results[0].Kind)
	})

	t.Run("Filter by non-existent type", func(t *testing.T) {
		filter := map[protocol.SymbolKind]bool{protocol.Interface: true}
		results := convertSymbolInformation(symbols, filter)
		assert.Len(t, results, 0)
	})
}

func TestFormatSymbolResults(t *testing.T) {
	t.Run("Empty results", func(t *testing.T) {
		result := formatSymbolResults([]symbolResult{})
		assert.Equal(t, "No symbols found.", result)
	})

	t.Run("Single symbol", func(t *testing.T) {
		symbols := []symbolResult{
			{
				Name:      "testFunc",
				Kind:      "Function",
				Location:  "/path/to/file.go:10:1",
				Container: "main",
				Detail:    "func testFunc() string",
			},
		}
		
		result := formatSymbolResults(symbols)
		expected := `Found 1 symbols:

**testFunc** (Function)
  Location: /path/to/file.go:10:1
  Container: main
  Detail: func testFunc() string

`
		assert.Equal(t, expected, result)
	})

	t.Run("Multiple symbols", func(t *testing.T) {
		symbols := []symbolResult{
			{
				Name:      "TestStruct",
				Kind:      "Struct",
				Location:  "/path/to/types.go:5:1",
				Container: "",
				Detail:    "",
			},
			{
				Name:      "Method1",
				Kind:      "Method",
				Location:  "/path/to/types.go:7:1",
				Container: "TestStruct",
				Detail:    "func (t *TestStruct) Method1()",
			},
		}
		
		result := formatSymbolResults(symbols)
		expected := `Found 2 symbols:

**TestStruct** (Struct)
  Location: /path/to/types.go:5:1

**Method1** (Method)
  Location: /path/to/types.go:7:1
  Container: TestStruct
  Detail: func (t *TestStruct) Method1()

`
		assert.Equal(t, expected, result)
	})
}

func TestFilterSymbolsByName(t *testing.T) {
	symbols := []symbolResult{
		{Name: "testFunction", Kind: "Function"},
		{Name: "TestClass", Kind: "Class"},
		{Name: "helper", Kind: "Function"},
		{Name: "test_method", Kind: "Method"},
		{Name: "getData", Kind: "Method"},
	}

	t.Run("No filter", func(t *testing.T) {
		result := filterSymbolsByName(symbols, nil)
		assert.Len(t, result, 5)
		assert.Equal(t, symbols, result)
	})

	t.Run("Filter by test prefix", func(t *testing.T) {
		regex := regexp.MustCompile("^test.*")
		result := filterSymbolsByName(symbols, regex)
		assert.Len(t, result, 2)
		assert.Equal(t, "testFunction", result[0].Name)
		assert.Equal(t, "test_method", result[1].Name)
	})

	t.Run("Filter by case insensitive Test", func(t *testing.T) {
		regex := regexp.MustCompile("(?i)test")
		result := filterSymbolsByName(symbols, regex)
		assert.Len(t, result, 3)
		assert.Equal(t, "testFunction", result[0].Name)
		assert.Equal(t, "TestClass", result[1].Name)
		assert.Equal(t, "test_method", result[2].Name)
	})

	t.Run("Filter by exact match", func(t *testing.T) {
		regex := regexp.MustCompile("^helper$")
		result := filterSymbolsByName(symbols, regex)
		assert.Len(t, result, 1)
		assert.Equal(t, "helper", result[0].Name)
	})

	t.Run("Filter with no matches", func(t *testing.T) {
		regex := regexp.MustCompile("^nonexistent.*")
		result := filterSymbolsByName(symbols, regex)
		assert.Len(t, result, 0)
	})
}