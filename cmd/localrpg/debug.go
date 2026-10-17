package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/darkliquid/localrpg/pkg/debugger"
	"github.com/darkliquid/localrpg/pkg/driver"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/storage"
)

type debugConfig struct {
	Scenario     string
	Headless     bool
	Port         int
	DebuggerPort int
	ReportDir    string
	// SystemID is the positional system id for `debug test-system`.
	SystemID string
	// Reference selects a shipped reference system rather than one on disk.
	Reference bool
}

func parseDebugArgs(args []string) (string, debugConfig, error) {
	if len(args) == 0 {
		return "", debugConfig{}, fmt.Errorf("subcommand required: 'test-run', 'test-system', or 'server'")
	}

	subcmd := args[0]
	fs := flag.NewFlagSet("debug "+subcmd, flag.ContinueOnError)
	var cfg debugConfig

	fs.StringVar(&cfg.Scenario, "scenario", "", "Path to YAML scenario file")
	fs.BoolVar(&cfg.Headless, "headless", true, "Run browser headlessly")
	fs.IntVar(&cfg.Port, "port", 8080, "App port")
	fs.IntVar(&cfg.DebuggerPort, "debugger-port", 8089, "Live debugger port")
	fs.StringVar(&cfg.ReportDir, "report-dir", "test-results", "Directory for test reports")
	fs.BoolVar(&cfg.Reference, "reference", false, "Test a shipped reference system")
	fs.StringVar(&cfg.SystemID, "system", "", "System id to test (positional also accepted)")

	if err := fs.Parse(args[1:]); err != nil {
		return "", debugConfig{}, err
	}
	if cfg.SystemID == "" && fs.NArg() > 0 {
		cfg.SystemID = fs.Arg(0)
	}

	return subcmd, cfg, nil
}

// handleDebugCommand runs automated scenario tests, system testing, or the debug server.
func handleDebugCommand(args []string) {
	code := runDebugCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runDebugCommand(args []string, stdout, stderr io.Writer) int {
	subcmd, cfg, err := parseDebugArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "Usage: localrpg debug <test-run|server> [flags]\nError: %v\n", err)
		return 1
	}

	collector := debugger.NewCollector(1000, 5000)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())

	dbgServer := debugger.NewServer(collector, fmt.Sprintf(":%d", cfg.DebuggerPort))

	switch subcmd {
	case "server":
		return serveDebugServers(stdout, stderr, cfg, dbgServer)
	case "test-system":
		return runTestSystem(cfg, stdout, stderr)
	case "test-run":
		return runDebugScenario(stdout, stderr, cfg, dbgServer)
	}
	return 0
}

// serveDebugServers runs the live debugger UI and the app side by side until a
// signal arrives.
func serveDebugServers(stdout, stderr io.Writer, cfg debugConfig, dbgServer *debugger.Server) int {
	fmt.Fprintln(stdout, "LocalRPG Debug Server mode active")
	fmt.Fprintf(stdout, "  • LocalRPG App GUI:   http://localhost:%d\n", cfg.Port)
	fmt.Fprintf(stdout, "  • Live Debugger UI:   http://localhost:%d\n", cfg.DebuggerPort)

	dbgHttpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.DebuggerPort),
		Handler: dbgServer.Handler(),
	}
	go func() {
		if err := dbgHttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stderr, "Debugger server error: %v\n", err)
		}
	}()

	svc := gui.NewService(".")
	defer svc.Close()
	defer func() { _ = storage.CloseGameStores() }()
	appHandler := gui.ProtectCrossOrigin(gui.NewServer(svc, gui.AssetHandler()))

	appHttpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: appHandler,
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := appHttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(stderr, "App server error: %v\n", err)
		}
	}()

	<-sigChan
	fmt.Fprintln(stdout, "\nShutting down debug servers...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = dbgHttpServer.Shutdown(shutdownCtx)
	_ = appHttpServer.Shutdown(shutdownCtx)
	return 0
}

// runDebugScenario drives one scenario against a running app, recording each step
// as it goes and writing the report the debugger UI reads.
func runDebugScenario(stdout, stderr io.Writer, cfg debugConfig, dbgServer *debugger.Server) int {
	if cfg.Scenario == "" {
		fmt.Fprintln(stderr, "Error: --scenario is required for test-run")
		return 1
	}

	f, err := os.Open(cfg.Scenario)
	if err != nil {
		fmt.Fprintf(stderr, "Open scenario: %v\n", err)
		return 1
	}
	defer f.Close()

	scenario, err := driver.ParseScenario(f)
	if err != nil {
		fmt.Fprintf(stderr, "Parse scenario: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Running Scenario: %s (%d steps)\n", scenario.Name, len(scenario.Steps))
	d := driver.New(driver.Config{
		// The app binds 127.0.0.1 explicitly, so address it the same way:
		// "localhost" can resolve to ::1 first on some machines.
		BaseURL:  fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
		Headless: cfg.Headless,
	})

	start := time.Now()
	records, runErr := d.Run(context.Background(), scenario, func(rec debugger.ActionRecord) {
		dbgServer.RecordAction(rec)
		statusStr := "[OK]"
		if rec.Status == "failed" {
			statusStr = "[FAIL]"
		}
		fmt.Fprintf(stdout, "%s Step %d: %s %s (%dms)\n", statusStr, rec.StepIndex, rec.ActionType, rec.Selector, rec.DurationMs)
	})

	report := debugger.TestReport{
		ScenarioName: scenario.Name,
		Description:  scenario.Description,
		StartTime:    start,
		EndTime:      time.Now(),
		DurationMs:   time.Since(start).Milliseconds(),
		Passed:       runErr == nil,
		Actions:      records,
		TotalActions: len(records),
	}

	reportPath := filepath.Join(cfg.ReportDir, fmt.Sprintf("report-%d.html", time.Now().Unix()))
	if err := debugger.ExportHTMLReport(report, reportPath); err == nil {
		fmt.Fprintf(stdout, "Report saved to %s\n", reportPath)
	}

	if runErr != nil {
		fmt.Fprintf(stderr, "Test run failed: %v\n", runErr)
		return 1
	}
	fmt.Fprintln(stdout, "Test run completed successfully!")
	return 0
}
