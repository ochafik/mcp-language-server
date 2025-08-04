package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/ochafik/mcp-language-server/internal/lsp"
	"github.com/ochafik/mcp-language-server/internal/protocol"
)

func GetSymbols(ctx context.Context, client *lsp.Client, symbolTypes []string, filePath *string, namePattern *string) (string, error) {
	toolsLogger.Debug("Getting symbols with types: %v, file: %v, namePattern: %v", symbolTypes, filePath, namePattern)

	var nameRegex *regexp.Regexp
	if namePattern != nil && *namePattern != "" {
		var err error
		nameRegex, err = regexp.Compile(*namePattern)
		if err != nil {
			return "", fmt.Errorf("invalid regex pattern: %v", err)
		}
	}

	symbolKindFilter := make(map[protocol.SymbolKind]bool)
	if len(symbolTypes) == 0 {
		// No kinds provided or empty array - use defaults
		defaultTypes := []string{"Class", "Interface", "Enum", "Struct", "Function", "Method", "Property", "Field"}
		for _, symbolType := range defaultTypes {
			kind := parseSymbolKind(symbolType)
			if kind != 0 {
				symbolKindFilter[kind] = true
			}
		}
	} else {
		// Specific kinds provided - filter by them
		for _, symbolType := range symbolTypes {
			kind := parseSymbolKind(symbolType)
			if kind != 0 {
				symbolKindFilter[kind] = true
			}
		}
		if len(symbolKindFilter) == 0 {
			return "", fmt.Errorf("no valid symbol types provided")
		}
	}

	var symbols []symbolResult
	var err error

	if filePath != nil {
		symbols, err = getDocumentSymbols(ctx, client, *filePath, symbolKindFilter)
	} else {
		symbols, err = getWorkspaceSymbols(ctx, client, "", symbolKindFilter)
	}

	if err != nil {
		return "", fmt.Errorf("failed to get symbols: %v", err)
	}

	filteredSymbols := filterSymbolsByName(symbols, nameRegex)

	return formatSymbolResults(filteredSymbols), nil
}

func filterSymbolsByName(symbols []symbolResult, nameRegex *regexp.Regexp) []symbolResult {
	if nameRegex == nil {
		return symbols
	}

	var filtered []symbolResult
	for _, symbol := range symbols {
		if nameRegex.MatchString(symbol.Name) {
			filtered = append(filtered, symbol)
		}
	}
	return filtered
}

type symbolResult struct {
	Name      string
	Kind      string
	Location  string
	Container string
	Parent    string
	Detail    string
}

func getDocumentSymbols(ctx context.Context, client *lsp.Client, filePath string, kindFilter map[protocol.SymbolKind]bool) ([]symbolResult, error) {
	params := protocol.DocumentSymbolParams{
		TextDocument: protocol.TextDocumentIdentifier{
			URI: protocol.DocumentUri("file://" + filePath),
		},
	}

	result, err := client.DocumentSymbol(ctx, params)
	if err != nil {
		return nil, err
	}

	var symbols []symbolResult
	
	switch v := result.Value.(type) {
	case []protocol.DocumentSymbol:
		symbols = flattenDocumentSymbols(v, "", kindFilter)
	case []protocol.SymbolInformation:
		symbols = convertSymbolInformation(v, kindFilter)
	default:
		toolsLogger.Debug("Unexpected document symbol result type: %T", v)
	}

	return symbols, nil
}

func getWorkspaceSymbols(ctx context.Context, client *lsp.Client, query string, kindFilter map[protocol.SymbolKind]bool) ([]symbolResult, error) {
	params := protocol.WorkspaceSymbolParams{
		Query: query,
	}

	result, err := client.Symbol(ctx, params)
	if err != nil {
		return nil, err
	}

	var symbols []symbolResult
	
	switch v := result.Value.(type) {
	case []protocol.WorkspaceSymbol:
		symbols = convertWorkspaceSymbols(v, kindFilter)
	case []protocol.SymbolInformation:
		symbols = convertSymbolInformation(v, kindFilter)
	default:
		toolsLogger.Debug("Unexpected workspace symbol result type: %T", v)
	}

	return symbols, nil
}

func flattenDocumentSymbols(docSymbols []protocol.DocumentSymbol, container string, kindFilter map[protocol.SymbolKind]bool) []symbolResult {
	return flattenDocumentSymbolsWithParent(docSymbols, container, "", kindFilter)
}

func flattenDocumentSymbolsWithParent(docSymbols []protocol.DocumentSymbol, container string, parent string, kindFilter map[protocol.SymbolKind]bool) []symbolResult {
	var results []symbolResult

	for _, symbol := range docSymbols {
		if len(kindFilter) == 0 || kindFilter[symbol.Kind] {
			kindName := protocol.TableKindMap[symbol.Kind]
			if kindName == "" {
				kindName = fmt.Sprintf("Unknown(%d)", symbol.Kind)
			}

			location := fmt.Sprintf("Line %d:%d-%d:%d", 
				symbol.Range.Start.Line+1, symbol.Range.Start.Character+1,
				symbol.Range.End.Line+1, symbol.Range.End.Character+1)

			results = append(results, symbolResult{
				Name:      symbol.Name,
				Kind:      kindName,
				Location:  location,
				Container: container,
				Parent:    parent,
				Detail:    symbol.Detail,
			})
		}

		if symbol.Children != nil {
			containerName := symbol.Name
			if container != "" {
				containerName = container + "." + symbol.Name
			}
			childResults := flattenDocumentSymbolsWithParent(symbol.Children, containerName, symbol.Name, kindFilter)
			results = append(results, childResults...)
		}
	}

	return results
}

func convertWorkspaceSymbols(workspaceSymbols []protocol.WorkspaceSymbol, kindFilter map[protocol.SymbolKind]bool) []symbolResult {
	var results []symbolResult

	for _, symbol := range workspaceSymbols {
		if len(kindFilter) == 0 || kindFilter[symbol.Kind] {
			kindName := protocol.TableKindMap[symbol.Kind]
			if kindName == "" {
				kindName = fmt.Sprintf("Unknown(%d)", symbol.Kind)
			}

			loc := symbol.GetLocation()
			var location string
			if loc.Range.Start.Line >= 0 {
				location = fmt.Sprintf("%s:%d:%d", 
					strings.TrimPrefix(string(loc.URI), "file://"),
					loc.Range.Start.Line+1, 
					loc.Range.Start.Character+1)
			} else {
				location = strings.TrimPrefix(string(loc.URI), "file://")
			}

			results = append(results, symbolResult{
				Name:      symbol.Name,
				Kind:      kindName,
				Location:  location,
				Container: symbol.ContainerName,
				Parent:    "",
				Detail:    "",
			})
		}
	}

	return results
}

func convertSymbolInformation(symbolInfos []protocol.SymbolInformation, kindFilter map[protocol.SymbolKind]bool) []symbolResult {
	var results []symbolResult

	for _, symbol := range symbolInfos {
		if len(kindFilter) == 0 || kindFilter[symbol.Kind] {
			kindName := protocol.TableKindMap[symbol.Kind]
			if kindName == "" {
				kindName = fmt.Sprintf("Unknown(%d)", symbol.Kind)
			}

			location := fmt.Sprintf("%s:%d:%d", 
				strings.TrimPrefix(string(symbol.Location.URI), "file://"),
				symbol.Location.Range.Start.Line+1, 
				symbol.Location.Range.Start.Character+1)

			results = append(results, symbolResult{
				Name:      symbol.Name,
				Kind:      kindName,
				Location:  location,
				Container: symbol.ContainerName,
				Parent:    "",
				Detail:    "",
			})
		}
	}

	return results
}

func parseSymbolKind(symbolType string) protocol.SymbolKind {
	switch strings.ToLower(symbolType) {
	case "file":
		return protocol.File
	case "module":
		return protocol.Module
	case "namespace":
		return protocol.Namespace
	case "package":
		return protocol.Package
	case "class":
		return protocol.Class
	case "method":
		return protocol.Method
	case "property":
		return protocol.Property
	case "field":
		return protocol.Field
	case "constructor":
		return protocol.Constructor
	case "enum":
		return protocol.Enum
	case "interface":
		return protocol.Interface
	case "function":
		return protocol.Function
	case "variable":
		return protocol.Variable
	case "constant":
		return protocol.Constant
	case "string":
		return protocol.String
	case "number":
		return protocol.Number
	case "boolean":
		return protocol.Boolean
	case "array":
		return protocol.Array
	case "object":
		return protocol.Object
	case "key":
		return protocol.Key
	case "null":
		return protocol.Null
	case "enummember":
		return protocol.EnumMember
	case "struct":
		return protocol.Struct
	case "event":
		return protocol.Event
	case "operator":
		return protocol.Operator
	case "typeparameter":
		return protocol.TypeParameter
	default:
		toolsLogger.Debug("Unknown symbol type: %s", symbolType)
		return 0
	}
}

func formatSymbolResults(symbols []symbolResult) string {
	if len(symbols) == 0 {
		return "No symbols found."
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Found %d symbols:\n\n", len(symbols)))

	for _, symbol := range symbols {
		output.WriteString(fmt.Sprintf("**%s** (%s)\n", symbol.Name, symbol.Kind))
		output.WriteString(fmt.Sprintf("  Location: %s\n", symbol.Location))
		
		if symbol.Container != "" {
			output.WriteString(fmt.Sprintf("  Container: %s\n", symbol.Container))
		}
		
		if symbol.Parent != "" {
			output.WriteString(fmt.Sprintf("  Parent: %s\n", symbol.Parent))
		}
		
		if symbol.Detail != "" {
			output.WriteString(fmt.Sprintf("  Detail: %s\n", symbol.Detail))
		}
		
		output.WriteString("\n")
	}

	return output.String()
}