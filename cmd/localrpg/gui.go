package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

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
	svc.SetVersion(Version)
	defer svc.Close()
	defer func() { _ = storage.CloseGameStores() }()

	telemetryProvider, err := telemetry.New(context.Background(), svc.Config().Telemetry, telemetry.BuildInfo{
		Version: Version,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting telemetry: %v\n", err)
		svc.Close()
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryProvider.Shutdown(shutdownCtx)
	}()

	// A batch speech job is async: if one was left running when the app closed,
	// collect it now in the background rather than making the user restart it.
	svc.ResumePendingBatches(context.Background())

	// os.Exit skips defers, so every exit path past this point drains the
	// service's background work (narration warm-up, enrichment, retro-summary)
	// before leaving.
	shutdown := func(code int) {
		svc.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = telemetryProvider.Shutdown(shutdownCtx)
		cancel()
		_ = storage.CloseGameStores()
		os.Exit(code)
	}

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
			shutdown(1)
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
			shutdown(1)
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
			shutdown(0)
		}()

		fmt.Printf("LocalRPG GUI daemon listening on Unix domain socket: %s (0 TCP ports)\n", sockPath)
		if !hasDisplay && !cfg.Headless {
			fmt.Println("Note: No desktop display detected ($DISPLAY / $WAYLAND_DISPLAY unset).")
		}
		if err := http.Serve(listener, handler); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Daemon failed: %v\n", err)
			shutdown(1)
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

	// macOS always shows a global menu bar, so it keeps a native menu (without
	// Wails' own Help > Learn More entry, which opens wails.io). On every other
	// platform the menu is drawn by the frontend and stays hidden until Alt
	// reveals it, so no native menu bar is attached: Wails v3 can only hide one
	// on Windows, and attaching it would leave it permanently visible on Linux.
	if runtime.GOOS == "darwin" {
		app.Menu.Set(localRPGApplicationMenu())
	}

	// A link in the app (the Help menu, the About dialog) cannot be opened by
	// the webview itself: Wails v3 installs no handler for a new window, so an
	// anchor click is a silent no-op on Linux. Routing it through the app's
	// Browser manager is what reaches the system browser.
	svc.SetURLOpener(app.Browser.OpenURL)

	// The desktop window gets a native directory chooser for exports. Browser and
	// socket modes have no dialog, so the UI falls back to a path field. Wails
	// dialogs must run on the application's main thread, and this callback is
	// reached from an HTTP handler goroutine, so the dialog is marshalled over.
	// A dismissed dialog is reported as a cancellation, not a failure.
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		return application.InvokeSyncWithResultAndError(func() (string, error) {
			if title == "" {
				title = "Choose a folder"
			}
			dialog := app.Dialog.OpenFile().
				CanChooseDirectories(true).
				CanChooseFiles(false).
				CanCreateDirectories(true).
				SetTitle(title)
			if defaultDir != "" {
				dialog = dialog.SetDirectory(defaultDir)
			}
			chosen, err := dialog.PromptForSingleSelection()
			if err != nil {
				return "", nil
			}
			return chosen, nil
		})
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:                      "LocalRPG",
		Width:                      1280,
		Height:                     800,
		MinWidth:                   900,
		MinHeight:                  600,
		URL:                        "/",
		BackgroundType:             application.BackgroundTypeTranslucent,
		DefaultContextMenuDisabled: false,
	})

	// With no native View menu, Developer Tools keeps its conventional
	// accelerator as a direct window binding so debugging stays reachable.
	window.RegisterKeyBinding("F12", func(application.Window) {
		window.OpenDevTools()
	})

	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Wails application failed: %v\n", err)
		shutdown(1)
	}
}

// localRPGApplicationMenu is the native menu macOS shows in the global menu bar.
// It is the platform default minus the Help menu, whose only entry ("Learn More")
// opens wails.io rather than anything about LocalRPG; the application's own Help
// and About live in the frontend menu. The About item the AppMenu role adds
// reports the application name and description, so it already describes LocalRPG.
func localRPGApplicationMenu() *application.Menu {
	menu := application.NewMenu()
	menu.AddRole(application.AppMenu)
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}
