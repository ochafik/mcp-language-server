package symbols_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ochafik/mcp-language-server/integrationtests/tests/common"
	"github.com/ochafik/mcp-language-server/integrationtests/tests/python/internal"
	"github.com/ochafik/mcp-language-server/internal/tools"
)

func TestGetSymbols(t *testing.T) {
	suite := internal.GetTestSuite(t)

	ctx, cancel := context.WithTimeout(suite.Context, 10*time.Second)
	defer cancel()

	tests := []struct {
		name         string
		symbolTypes  []string
		filePath     *string
		snapshotName string
		expectedText string
	}{
		{
			name:         "DocumentAllSymbols",
			symbolTypes:  []string{"Class", "Interface", "Enum", "Struct", "Function", "Method", "Property", "Field"},
			filePath:     stringPtr(suite.WorkspaceDir + "/main.py"),
			snapshotName: "document_all",
			expectedText: "Found",
		},
		{
			name:         "DocumentClassesOnly",
			symbolTypes:  []string{"Class"},
			filePath:     stringPtr(suite.WorkspaceDir + "/main.py"),
			snapshotName: "document_classes",
			expectedText: "TestClass",
		},
		{
			name:         "DocumentFunctionsOnly",
			symbolTypes:  []string{"Function"},
			filePath:     stringPtr(suite.WorkspaceDir + "/main.py"),
			snapshotName: "document_functions",
			expectedText: "test_function",
		},
		{
			name:         "DocumentMultipleTypes",
			symbolTypes:  []string{"Class", "Method"},
			filePath:     stringPtr(suite.WorkspaceDir + "/main.py"),
			snapshotName: "document_class_method",
			expectedText: "TestClass",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := tools.GetSymbols(ctx, suite.Client, test.symbolTypes, test.filePath, nil)

			if err != nil {
				t.Fatalf("GetSymbols failed: %v", err)
			}

			if !strings.Contains(result, test.expectedText) {
				t.Errorf("Expected result to contain '%s', got: %s", test.expectedText, result)
			}

			common.SnapshotTest(t, "python", "symbols", test.snapshotName, result)
		})
	}
}

func TestGetSymbolsInvalidTypes(t *testing.T) {
	suite := internal.GetTestSuite(t)

	ctx, cancel := context.WithTimeout(suite.Context, 5*time.Second)
	defer cancel()

	_, err := tools.GetSymbols(ctx, suite.Client, []string{"InvalidType"}, nil, nil)
	if err == nil {
		t.Error("Expected error for invalid symbol types")
	}
}

func TestGetSymbolsWithNamePattern(t *testing.T) {
	suite := internal.GetTestSuite(t)

	ctx, cancel := context.WithTimeout(suite.Context, 10*time.Second)
	defer cancel()

	tests := []struct {
		name         string
		namePattern  string
		snapshotName string
		expectedText string
	}{
		{
			name:         "TestPrefixPattern",
			namePattern:  "^test.*",
			snapshotName: "name_pattern_test_prefix",
			expectedText: "test_function",
		},
		{
			name:         "ClassPattern",
			namePattern:  ".*Class$",
			snapshotName: "name_pattern_class_suffix",
			expectedText: "TestClass",
		},
		{
			name:         "MethodPattern",
			namePattern:  ".*method.*",
			snapshotName: "name_pattern_method_contains",
			expectedText: "test_method",
		},
	}

	filePath := suite.WorkspaceDir + "/main.py"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := tools.GetSymbols(ctx, suite.Client, []string{"Class", "Interface", "Enum", "Struct", "Function", "Method", "Property", "Field"}, &filePath, &test.namePattern)

			if err != nil {
				t.Fatalf("GetSymbols failed: %v", err)
			}

			if !strings.Contains(result, test.expectedText) {
				t.Errorf("Expected result to contain '%s', got: %s", test.expectedText, result)
			}

			common.SnapshotTest(t, "python", "symbols", test.snapshotName, result)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}