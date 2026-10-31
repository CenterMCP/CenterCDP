package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rvarun11/sqlite-mcp/internal/cdp"
	"github.com/rvarun11/sqlite-mcp/internal/config"
	"github.com/rvarun11/sqlite-mcp/internal/handlers"
	"github.com/rvarun11/sqlite-mcp/internal/logger"
	"github.com/rvarun11/sqlite-mcp/internal/repository"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	dbPath        string
	listenAddress string
	endpointPath  string
	dataDirectory string
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "sqlite-mcp",
		Short: "SQLite MCP Server - A Model Context Protocol server for SQLite operations",
		Long:  `SQLite MCP Server provides a standardized interface for SQLite database operations through the Model Context Protocol (MCP). It supports schema introspection, query execution, and database modifications.`,
		Run:   runServer,
	}

	rootCmd.Flags().StringVarP(&dbPath, "database", "d", "", "Path to SQLite database file (required)")
	rootCmd.Flags().Bool("debug", false, "Enable debug mode")
	rootCmd.Flags().StringVar(&listenAddress, "address", defaultListenAddress(), "Streamable HTTP listen address")
	rootCmd.Flags().StringVar(&endpointPath, "endpoint", "/mcp", "Streamable HTTP endpoint path")
	rootCmd.Flags().StringVar(&dataDirectory, "data-dir", "", "Directory for managed customer databases (default: next to --database)")

	err := rootCmd.MarkFlagRequired("database")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marking database flag as required: %v\n", err)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runServer(cmd *cobra.Command, args []string) {
	// Initialize configuration
	cfg, err := config.NewConfig(cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	logger, err := logger.NewLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer syncLogger(logger)

	logger.Infof("Starting SQLite MCP Server: %v", dbPath)

	// Initialize database
	repo, err := repository.NewSQLiteDB(cfg.DatabasePath, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize database: %v", err)
	}
	defer repo.Close()

	if dataDirectory == "" {
		base := strings.TrimSuffix(filepath.Base(cfg.DatabasePath), filepath.Ext(cfg.DatabasePath))
		dataDirectory = filepath.Join(filepath.Dir(cfg.DatabasePath), base+"-customer-databases")
	}
	cdpManager, err := cdp.NewManager(cfg.DatabasePath+".registry.db", dataDirectory, cfg.DatabasePath, logger)
	if err != nil {
		logger.Fatalf("Failed to initialize CDP database manager: %v", err)
	}
	defer cdpManager.Close()
	cdpService := cdp.NewService(cdpManager, logger)
	cdpHandler := handlers.NewCDPHandler(cdpService)
	workerCtx, stopWorker := context.WithCancel(context.Background())
	defer stopWorker()
	go cdpService.RunJourneyWorker(workerCtx)

	// Initialize MCP handler
	mcpHandler := handlers.NewMCPHandler(repo, logger)

	mcpServer := server.NewMCPServer(
		"sqlite-mcp",
		"1.0.0",
	)

	// Get Schema Tool - No parameters needed
	listTablesTool := mcp.NewTool("get_schema",
		mcp.WithDescription("List all tables in the SQLite database with their schema information including columns, types, constraints, and indexes"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)
	mcpServer.AddTool(listTablesTool, mcpHandler.GetSchema)

	// Query Database Tool
	queryDatabaseTool := mcp.NewTool("query",
		mcp.WithDescription("Execute SELECT queries against the SQLite database. Only SELECT, WITH, and EXPLAIN queries are allowed."),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("SQL SELECT query to execute"),
			mcp.MinLength(1),
			mcp.MaxLength(10000),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
	)
	mcpServer.AddTool(queryDatabaseTool, mcpHandler.Query)

	// Execute Database Tool
	executeDatabaseTool := mcp.NewTool("execute",
		mcp.WithDescription("Execute DDL/DML operations (INSERT, UPDATE, DELETE, CREATE, ALTER, DROP, etc.) against the SQLite database. SELECT queries are not allowed - use queryDatabase instead."),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("SQL statement to execute (non-SELECT operations only)"),
			mcp.MinLength(1),
			mcp.MaxLength(10000),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
	)
	mcpServer.AddTool(executeDatabaseTool, mcpHandler.Execute)

	handlers.RegisterCDPTools(mcpServer, cdpHandler)

	endpointPath, err = normalizeEndpointPath(endpointPath)
	if err != nil {
		logger.Fatalf("Invalid HTTP endpoint: %v", err)
	}

	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:              listenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	streamableHTTPServer := server.NewStreamableHTTPServer(
		mcpServer,
		server.WithStreamableHTTPServer(httpServer),
	)
	mux.Handle(endpointPath, streamableHTTPServer)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	go func() {
		sig := <-sigChan
		logger.Infof("Received %s, gracefully shutting down...", sig)
		stopWorker()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := streamableHTTPServer.Shutdown(ctx); err != nil {
			logger.Errorf("Failed to shut down Streamable HTTP server: %v", err)
		}
	}()

	logger.Infof("SQLite MCP Server listening on http://%s%s", listenAddress, endpointPath)
	if err := streamableHTTPServer.Start(listenAddress); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("Streamable HTTP server failed: %v", err)
	}

	logger.Info("SQLite MCP Server stopped")
}

func defaultListenAddress() string {
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	return net.JoinHostPort(host, port)
}

func normalizeEndpointPath(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", errors.New("endpoint path cannot be empty")
	}
	if strings.ContainsAny(endpoint, "?#") {
		return "", errors.New("endpoint path cannot contain a query or fragment")
	}

	normalized := "/" + strings.Trim(endpoint, "/")
	if normalized == "/health" {
		return "", errors.New("endpoint path /health is reserved")
	}
	return normalized, nil
}

func syncLogger(logger *zap.SugaredLogger) {
	if err := logger.Sync(); err != nil && !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.ENOTTY) {
		fmt.Fprintf(os.Stderr, "Failed to sync logger: %v\n", err)
	}
}
