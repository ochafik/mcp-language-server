package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ochafik/mcp-language-server/internal/logging"
	"github.com/ochafik/mcp-language-server/internal/lsp"
	"github.com/ochafik/mcp-language-server/internal/watcher"
	"github.com/mark3labs/mcp-go/server"
)

// Create a logger for the core component
var coreLogger = logging.NewLogger(logging.Core)

// stringSlice implements flag.Value for collecting multiple string flags
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

type config struct {
	workspaceDir     string
	lspCommand       string
	lspArgs          []string
	preopenPatterns  []string
}

type mcpServer struct {
	config           config
	lspClient        *lsp.Client
	mcpServer        *server.MCPServer
	ctx              context.Context
	cancelFunc       context.CancelFunc
	workspaceWatcher *watcher.WorkspaceWatcher
}

func parseConfig() (*config, error) {
	cfg := &config{}
	var patterns stringSlice
	
	flag.StringVar(&cfg.workspaceDir, "workspace", "", "Path to workspace directory")
	flag.StringVar(&cfg.lspCommand, "lsp", "", "LSP command to run (args should be passed after --)")
	flag.Var(&patterns, "preopen-files-matching", "Glob pattern for files to pre-open (can be specified multiple times, e.g., --preopen-files-matching '*.h' --preopen-files-matching '*.hpp')")
	flag.Parse()
	
	cfg.preopenPatterns = []string(patterns)

	// Get remaining args after -- as LSP arguments
	cfg.lspArgs = flag.Args()

	// Validate workspace directory
	if cfg.workspaceDir == "" {
		return nil, fmt.Errorf("workspace directory is required")
	}

	workspaceDir, err := filepath.Abs(cfg.workspaceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for workspace: %v", err)
	}
	cfg.workspaceDir = workspaceDir

	if _, err := os.Stat(cfg.workspaceDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("workspace directory does not exist: %s", cfg.workspaceDir)
	}

	// Validate LSP command
	if cfg.lspCommand == "" {
		return nil, fmt.Errorf("LSP command is required")
	}

	if _, err := exec.LookPath(cfg.lspCommand); err != nil {
		return nil, fmt.Errorf("LSP command not found: %s", cfg.lspCommand)
	}

	return cfg, nil
}

// findFilesToPreopen discovers files matching the given glob patterns
func findFilesToPreopen(workspaceDir string, patterns []string, maxFiles int) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	
	var allFiles []string
	seenFiles := make(map[string]bool)
	
	for _, pattern := range patterns {
		// Make pattern relative to workspace directory
		fullPattern := filepath.Join(workspaceDir, pattern)
		
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			coreLogger.Error("Failed to glob pattern %s: %v", pattern, err)
			continue
		}
		
		for _, match := range matches {
			// Convert to relative path and avoid duplicates
			relPath, err := filepath.Rel(workspaceDir, match)
			if err != nil {
				continue
			}
			
			// Skip if already seen or if it's a directory
			if seenFiles[relPath] {
				continue
			}
			
			if info, err := os.Stat(match); err != nil || info.IsDir() {
				continue
			}
			
			seenFiles[relPath] = true
			allFiles = append(allFiles, match)
			
			// Respect maxFiles limit
			if len(allFiles) >= maxFiles {
				coreLogger.Info("Reached maximum file limit (%d), stopping file discovery", maxFiles)
				return allFiles, nil
			}
		}
	}
	
	return allFiles, nil
}

// preopenFiles opens files in the LSP client to trigger indexing
func (s *mcpServer) preopenFiles() error {
	if len(s.config.preopenPatterns) == 0 {
		return nil
	}
	
	coreLogger.Info("Pre-opening files matching patterns: %v", s.config.preopenPatterns)
	
	// Find files to open (limit to 300 to avoid overwhelming clangd)
	filesToOpen, err := findFilesToPreopen(s.config.workspaceDir, s.config.preopenPatterns, 300)
	if err != nil {
		return fmt.Errorf("failed to find files to preopen: %v", err)
	}
	
	if len(filesToOpen) == 0 {
		coreLogger.Info("No files found matching patterns")
		return nil
	}
	
	coreLogger.Info("Pre-opening %d files for LSP indexing", len(filesToOpen))
	
	// Open files with timeout
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	
	openedCount := 0
	for _, filePath := range filesToOpen {
		if err := s.lspClient.OpenFile(ctx, filePath); err != nil {
			coreLogger.Error("Failed to open file %s: %v", filePath, err)
			continue
		}
		openedCount++
		
		// Add small delay to avoid overwhelming the LSP server
		if openedCount%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}
	
	coreLogger.Info("Successfully opened %d files", openedCount)
	
	// Give clangd some time to process the files
	if openedCount > 0 {
		coreLogger.Info("Waiting 2 seconds for LSP indexing...")
		time.Sleep(2 * time.Second)
	}
	
	return nil
}

func newServer(config *config) (*mcpServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &mcpServer{
		config:     *config,
		ctx:        ctx,
		cancelFunc: cancel,
	}, nil
}

func (s *mcpServer) initializeLSP() error {
	if err := os.Chdir(s.config.workspaceDir); err != nil {
		return fmt.Errorf("failed to change to workspace directory: %v", err)
	}

	client, err := lsp.NewClient(s.config.lspCommand, s.config.lspArgs...)
	if err != nil {
		return fmt.Errorf("failed to create LSP client: %v", err)
	}
	s.lspClient = client
	s.workspaceWatcher = watcher.NewWorkspaceWatcher(client)

	initResult, err := client.InitializeLSPClient(s.ctx, s.config.workspaceDir)
	if err != nil {
		return fmt.Errorf("initialize failed: %v", err)
	}

	coreLogger.Debug("Server capabilities: %+v", initResult.Capabilities)

	go s.workspaceWatcher.WatchWorkspace(s.ctx, s.config.workspaceDir)
	return client.WaitForServerReady(s.ctx)
}

func (s *mcpServer) start() error {
	if err := s.initializeLSP(); err != nil {
		return err
	}

	// Pre-open files if patterns are specified to help with LSP indexing
	if err := s.preopenFiles(); err != nil {
		coreLogger.Error("Failed to pre-open files: %v", err)
		// Don't fail startup, just log the error
	}

	s.mcpServer = server.NewMCPServer(
		"MCP Language Server",
		"v0.0.2",
		server.WithLogging(),
		server.WithRecovery(),
	)

	err := s.registerTools()
	if err != nil {
		return fmt.Errorf("tool registration failed: %v", err)
	}

	return server.ServeStdio(s.mcpServer)
}

func main() {
	coreLogger.Info("MCP Language Server starting")

	done := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	config, err := parseConfig()
	if err != nil {
		coreLogger.Fatal("%v", err)
	}

	server, err := newServer(config)
	if err != nil {
		coreLogger.Fatal("%v", err)
	}

	// Parent process monitoring channel
	parentDeath := make(chan struct{})

	// Monitor parent process termination
	// Claude desktop does not properly kill child processes for MCP servers
	go func() {
		ppid := os.Getppid()
		coreLogger.Debug("Monitoring parent process: %d", ppid)

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				currentPpid := os.Getppid()
				if currentPpid != ppid && (currentPpid == 1 || ppid == 1) {
					coreLogger.Info("Parent process %d terminated (current ppid: %d), initiating shutdown", ppid, currentPpid)
					close(parentDeath)
					return
				}
			case <-done:
				return
			}
		}
	}()

	// Handle shutdown triggers
	go func() {
		select {
		case sig := <-sigChan:
			coreLogger.Info("Received signal %v in PID: %d", sig, os.Getpid())
			cleanup(server, done)
		case <-parentDeath:
			coreLogger.Info("Parent death detected, initiating shutdown")
			cleanup(server, done)
		}
	}()

	if err := server.start(); err != nil {
		coreLogger.Error("Server error: %v", err)
		cleanup(server, done)
		os.Exit(1)
	}

	<-done
	coreLogger.Info("Server shutdown complete for PID: %d", os.Getpid())
	os.Exit(0)
}

func cleanup(s *mcpServer, done chan struct{}) {
	coreLogger.Info("Cleanup initiated for PID: %d", os.Getpid())

	// Create a context with timeout for shutdown operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.lspClient != nil {
		coreLogger.Info("Closing open files")
		s.lspClient.CloseAllFiles(ctx)

		// Create a shorter timeout context for the shutdown request
		shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 500*time.Millisecond)
		defer shutdownCancel()

		// Run shutdown in a goroutine with timeout to avoid blocking if LSP doesn't respond
		shutdownDone := make(chan struct{})
		go func() {
			coreLogger.Info("Sending shutdown request")
			if err := s.lspClient.Shutdown(shutdownCtx); err != nil {
				coreLogger.Error("Shutdown request failed: %v", err)
			}
			close(shutdownDone)
		}()

		// Wait for shutdown with timeout
		select {
		case <-shutdownDone:
			coreLogger.Info("Shutdown request completed")
		case <-time.After(1 * time.Second):
			coreLogger.Warn("Shutdown request timed out, proceeding with exit")
		}

		coreLogger.Info("Sending exit notification")
		if err := s.lspClient.Exit(ctx); err != nil {
			coreLogger.Error("Exit notification failed: %v", err)
		}

		coreLogger.Info("Closing LSP client")
		if err := s.lspClient.Close(); err != nil {
			coreLogger.Error("Failed to close LSP client: %v", err)
		}
	}

	// Send signal to the done channel
	select {
	case <-done: // Channel already closed
	default:
		close(done)
	}

	coreLogger.Info("Cleanup completed for PID: %d", os.Getpid())
}
