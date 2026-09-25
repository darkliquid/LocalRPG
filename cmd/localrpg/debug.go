package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/darkliquid/localrpg/pkg/debugger"
	"github.com/darkliquid/localrpg/pkg/driver"
)

type debugConfig struct {
	Scenario     string
	Headless     bool
	Port         int
	DebuggerPort int
	ReportDir    string
}

func parseDebugArgs(args []string) (string, debugConfig, error) {
	if len(args) == 0 {
		return "", debugConfig{}, fmt.Errorf("subcommand required: 'test-run' or 'server'")
	}

	subcmd := args[0]
	fs := flag.NewFlagSet("debug "+subcmd, flag.ContinueOnError)
	var cfg debugConfig

	fs.StringVar(&cfg.Scenario, "scenario", "", "Path to YAML scenario file")
	fs.BoolVar(&cfg.Headless, "headless", true, "Run browser headlessly")
	fs.IntVar(&cfg.Port, "port", 8080, "App port")
	fs.IntVar(&cfg.DebuggerPort, "debugger-port", 8089, "Live debugger port")
	fs.StringVar(&cfg.ReportDir, "report-dir", "test-results", "Directory for test reports")

	if err := fs.Parse(args[1:]); err != nil {
		return "", debugConfig{}, err
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
		fmt.Printf("Starting LocalRPG Debugger Server on http://localhost:%d\n", cfg.DebuggerPort)
		server := &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.DebuggerPort),
			Handler: dbgServer.Handler(),
		}
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Debugger server failed: %v\n", err)
			os.Exit(1)
		}

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
			BaseURL:  fmt.Sprintf("http://localhost:%d", cfg.Port),
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
