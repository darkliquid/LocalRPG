package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/darkliquid/localrpg/pkg/desktop"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
)

// bootDesktop starts the in-process shirei GUI. When png is set it renders a
// single frame to that path and returns instead of opening a window.
func bootDesktop(dir, png string) int {
	svc := gui.NewService(dir)
	defer func() { _ = storage.CloseGameStores() }()

	telemetryProvider, err := telemetry.New(context.Background(), svc.Config().Telemetry, telemetry.BuildInfo{
		Version: Version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting telemetry: %v\n", err)
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryProvider.Shutdown(shutdownCtx)
	}()

	// The resolver owns where caches live, so the sink follows it rather than
	// duplicating the relative-path resolution the service already did. The
	// telemetry bridge wraps it so events also reach OTel when enabled.
	localLogger := buildTraceLogger(svc.Config(), os.Args, svc.GetResolver().CacheDir())
	svc.SetLogger(telemetryProvider.Logger(localLogger))

	if err := desktop.Run(desktop.Config{Dir: dir, PNGPath: png, Service: svc}); err != nil {
		fmt.Fprintf(os.Stderr, "GUI failed: %v\n", err)
		return 1
	}
	return 0
}
