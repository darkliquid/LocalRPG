package main

import (
	"context"
	"flag"
	"fmt"
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

func handleDebugCommand(args []string) {
	subcmd, cfg, err := parseDebugArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Usage: localrpg debug <test-run|server> [flags]\nError: %v\n", err)
		os.Exit(1)
	}

	collector := debugger.NewCollector(1000, 5000)
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(collector))
	otel.SetTracerProvider(tp)
	defer tp.Shutdown(context.Background())

	dbgServer := debugger.NewServer(collector, fmt.Sprintf(":%d", cfg.DebuggerPort))

	switch subcmd {
	case "server":
		fmt.Println("LocalRPG Debug Server mode active")
		fmt.Printf("  • LocalRPG App GUI:   http://localhost:%d\n", cfg.Port)
		fmt.Printf("  • Live Debugger UI:   http://localhost:%d\n", cfg.DebuggerPort)

		dbgHttpServer := &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.DebuggerPort),
			Handler: dbgServer.Handler(),
		}
		go func() {
			if err := dbgHttpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "Debugger server error: %v\n", err)
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
				fmt.Fprintf(os.Stderr, "App server error: %v\n", err)
			}
		}()

		<-sigChan
		fmt.Println("\nShutting down debug servers...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = dbgHttpServer.Shutdown(shutdownCtx)
		_ = appHttpServer.Shutdown(shutdownCtx)

	case "test-system":
		os.Exit(runTestSystem(cfg))

	case "test-run":
		if cfg.Scenario == "" {
			fmt.Fprintln(os.Stderr, "Error: --scenario is required for test-run")
			os.Exit(1)
		}

		f, err := os.Open(cfg.Scenario)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Open scenario: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		scenario, err := driver.ParseScenario(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Parse scenario: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Running Scenario: %s (%d steps)\n", scenario.Name, len(scenario.Steps))
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
			fmt.Printf("%s Step %d: %s %s (%dms)\n", statusStr, rec.StepIndex, rec.ActionType, rec.Selector, rec.DurationMs)
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
			fmt.Printf("Report saved to %s\n", reportPath)
		}

		if runErr != nil {
			fmt.Fprintf(os.Stderr, "Test run failed: %v\n", runErr)
			os.Exit(1)
		}
		fmt.Println("Test run completed successfully!")
	}
}
