package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/darkliquid/localrpg/pkg/desktop"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/storage"
	"github.com/darkliquid/localrpg/pkg/telemetry"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type guiConfig struct {
	Dir        string
	Port       int
	SocketPath string
	WebMode    bool
	Headless   bool
}

func parseGUIConfig(args []string) (*guiConfig, error) {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: localrpg gui [flags]")
		fmt.Fprintln(os.Stderr, "Launch desktop GUI or browser app")
		fs.PrintDefaults()
	}
	port := fs.Int("port", 0, "Explicit TCP port to bind (opt-in web server)")
	socket := fs.String("socket", "", "Unix domain socket path (defaults to user runtime socket)")
	dir := fs.String("dir", ".", "Project root directory")
	headless := fs.Bool("headless", false, "Run in headless mode (listen on Unix domain socket)")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	cfg := &guiConfig{
		Dir:        *dir,
		Port:       *port,
		SocketPath: *socket,
		WebMode:    *port > 0,
		Headless:   *headless,
	}
	return cfg, nil
}

func handleGUICommand(args []string) {
	cfg, err := parseGUIConfig(args)
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(1)
	}

	svc := gui.NewService(cfg.Dir)
	defer func() { _ = storage.CloseGameStores() }()

	// Transitional escape hatch: run the in-process shirei GUI instead of the
	// Wails window or the HTTP daemon. Removed at teardown.
	if os.Getenv("LOCALRPG_UI") == "shirei" {
		if err := desktop.Run(desktop.Config{Dir: cfg.Dir, Service: svc}); err != nil {
			fmt.Fprintf(os.Stderr, "shirei GUI failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	telemetryProvider, err := telemetry.New(context.Background(), svc.Config().Telemetry, telemetry.BuildInfo{
		Version: Version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting telemetry: %v\n", err)
		os.Exit(1)
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

	handler := gui.ProtectCrossOrigin(gui.NewServer(svc, gui.AssetHandler()))

	// 1. Explicit TCP Web Mode
	if cfg.WebMode {
		addr := fmt.Sprintf("127.0.0.1:%d", cfg.Port)
		fmt.Printf("Starting LocalRPG Web GUI on http://%s\n", addr)
		if err := http.ListenAndServe(addr, handler); err != nil {
			fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 2. Explicit or Headless Unix Domain Socket Mode
	hasDisplay := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	if cfg.Headless || cfg.SocketPath != "" || !hasDisplay {
		sockPath := cfg.SocketPath
		if sockPath == "" {
			sockPath = gui.DefaultSocketPath()
		}

		listener, err := gui.ListenUnix(sockPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Socket error: %v\n", err)
			os.Exit(1)
		}
		defer listener.Close()
		defer os.Remove(sockPath)

		// Handle graceful shutdown signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\nShutting down LocalRPG GUI socket daemon...")
			_ = listener.Close()
			_ = os.Remove(sockPath)
			os.Exit(0)
		}()

		fmt.Printf("LocalRPG GUI daemon listening on Unix domain socket: %s (0 TCP ports)\n", sockPath)
		if !hasDisplay && !cfg.Headless {
			fmt.Println("Note: No desktop display detected ($DISPLAY / $WAYLAND_DISPLAY unset).")
		}
		if err := http.Serve(listener, handler); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Daemon failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// 3. Default: Native Wails v3 Desktop Window (Zero-TCP)
	app := application.New(application.Options{
		Name:        "LocalRPG",
		Description: "Local-First LLM Tabletop RPG Client",
		Assets: application.AssetOptions{
			Handler: handler,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "LocalRPG",
		Width:          1280,
		Height:         800,
		MinWidth:       900,
		MinHeight:      600,
		URL:            "/",
		BackgroundType: application.BackgroundTypeTranslucent,
	})

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Wails application failed: %v\n", err)
		os.Exit(1)
	}
}
