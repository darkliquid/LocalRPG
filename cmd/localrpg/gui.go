package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
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

func parseGUIConfig(args []string, stderr io.Writer) (*guiConfig, error) {
	fs := flag.NewFlagSet("gui", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: localrpg gui [flags]")
		fmt.Fprintln(stderr, "Launch desktop GUI or browser app")
		fs.PrintDefaults()
	}
	port := fs.Int("port", 0, "Explicit TCP port to bind (opt-in web server)")
	socket := fs.String("socket", "", "Unix domain socket path (defaults to user runtime socket)")
	dir := fs.String("dir", ".", "Project root directory")
	headless := fs.Bool("headless", false, "Run in headless mode (listen on Unix domain socket)")
	// The trace level is read from the argument list by buildTraceLogger, so the
	// flag is declared here only to keep the parser from rejecting it.
	fs.Var(traceFlag{}, "trace", "Trace level: off, summary, or full (a bare --trace means full)")

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

// handleGUICommand launches the desktop GUI window or web/socket server mode.
func handleGUICommand(args []string) {
	code := runGUICommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runGUICommand(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseGUIConfig(args, stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		fmt.Fprintf(stderr, "Flag error: %v\n", err)
		return 1
	}

	svc := gui.NewService(cfg.Dir)
	svc.SetVersion(Version)
	defer svc.Close()
	defer func() { _ = storage.CloseGameStores() }()

	telemetryProvider, err := telemetry.New(context.Background(), svc.Config().Telemetry, telemetry.BuildInfo{
		Version: Version,
	})
	if err != nil {
		fmt.Fprintf(stderr, "Error starting telemetry: %v\n", err)
		svc.Close()
		return 1
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = telemetryProvider.Shutdown(shutdownCtx)
	}()

	// A batch speech job is async: if one was left running when the app closed,
	// collect it now in the background rather than making the user restart it.
	svc.ResumePendingBatches(context.Background())

	// Every exit path past this point drains the service's background work
	// (narration warm-up, enrichment, retro-summary) before leaving, which the
	// deferred call below does now that the command returns a code instead of
	// reaching the process directly.
	defer func() {
		svc.Close()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = telemetryProvider.Shutdown(shutdownCtx)
		cancel()
		_ = storage.CloseGameStores()
	}()

	// The resolver owns where caches live, so the sink follows it rather than
	// duplicating the relative-path resolution the service already did. The
	// telemetry bridge wraps it so events also reach OTel when enabled.
	localLogger := buildTraceLogger(svc.Config(), os.Args, svc.GetResolver().CacheDir())
	svc.SetLogger(telemetryProvider.Logger(localLogger))

	handler := gui.ProtectCrossOrigin(gui.NewServer(svc, gui.AssetHandler()))

	// Three ways to serve the same app, in the order of how much of the machine
	// they claim: an explicit port, a socket, or the native window.
	switch {
	case cfg.WebMode:
		return serveWebMode(stdout, stderr, cfg.Port, handler)
	case cfg.Headless || cfg.SocketPath != "" || !hasDisplay():
		return serveSocketDaemon(stdout, stderr, cfg, handler)
	default:
		return serveDesktopApp(stdout, stderr, svc, handler)
	}
}

// hasDisplay reports whether a desktop window has somewhere to appear.
func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// serveWebMode serves the app over loopback TCP, which a browser can reach.
func serveWebMode(stdout, stderr io.Writer, port int, handler http.Handler) int {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Fprintf(stdout, "Starting LocalRPG Web GUI on http://%s\n", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(stderr, "Server failed: %v\n", err)
		return 1
	}
	return 0
}

// serveSocketDaemon serves the app over a Unix domain socket, which is what an
// explicit --socket, --headless, or a host with no display gets. The socket is
// unlinked on the way out, and a signal closes the listener rather than reaching
// the process.
func serveSocketDaemon(stdout, stderr io.Writer, cfg *guiConfig, handler http.Handler) int {
	sockPath := cfg.SocketPath
	if sockPath == "" {
		sockPath = gui.DefaultSocketPath()
	}

	listener, err := gui.ListenUnix(sockPath)
	if err != nil {
		fmt.Fprintf(stderr, "Socket error: %v\n", err)
		return 1
	}
	defer listener.Close()
	defer os.Remove(sockPath)

	// Handle graceful shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	// The signal closes the listener, which is what unblocks Serve below. The
	// flag tells that apart from a server failure, so a clean Ctrl-C is not
	// reported as one; the deferred drain runs when this command returns.
	var shuttingDown atomic.Bool
	go func() {
		<-sigChan
		fmt.Fprintln(stdout, "\nShutting down LocalRPG GUI socket daemon...")
		shuttingDown.Store(true)
		_ = listener.Close()
		_ = os.Remove(sockPath)
	}()

	fmt.Fprintf(stdout, "LocalRPG GUI daemon listening on Unix domain socket: %s (0 TCP ports)\n", sockPath)
	if !hasDisplay() && !cfg.Headless {
		fmt.Fprintln(stdout, "Note: No desktop display detected ($DISPLAY / $WAYLAND_DISPLAY unset).")
	}

	if err := http.Serve(listener, handler); err != nil && err != http.ErrServerClosed && !shuttingDown.Load() {
		fmt.Fprintf(stderr, "Daemon failed: %v\n", err)
		return 1
	}
	return 0
}

// serveDesktopApp serves the app in a native Wails v3 window, which is the
// default because it binds no TCP port at all.
func serveDesktopApp(stdout, stderr io.Writer, svc *gui.Service, handler http.Handler) int {
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

	installDirectoryPicker(app, svc, window)
	installSaveFilePicker(app, svc, window)

	// With no native View menu, Developer Tools keeps its conventional
	// accelerator as a direct window binding so debugging stays reachable.
	window.RegisterKeyBinding("F12", func(application.Window) {
		window.OpenDevTools()
	})

	if err := app.Run(); err != nil {
		fmt.Fprintf(stderr, "Wails application failed: %v\n", err)
		return 1
	}
	return 0
}

// installDirectoryPicker gives the desktop window a native directory chooser.
// Browser and socket modes have no dialog, so the UI falls back to a path field.
//
// PromptForSingleSelection marshals the dialog onto the application's main
// thread itself, so this closure is not wrapped in another InvokeSync: a
// second hop would run the inner one from inside a main-thread callback,
// where the idle it posts can never be serviced. The dialog is attached to
// the window so it is modal to it rather than a free-floating toplevel. A
// dismissed dialog is reported as a cancellation, not a failure.
func installDirectoryPicker(app *application.App, svc *gui.Service, window *application.WebviewWindow) {
	svc.SetDirectoryPicker(func(title, defaultDir string) (string, error) {
		if title == "" {
			title = "Choose a folder"
		}
		dialog := app.Dialog.OpenFile().
			CanChooseDirectories(true).
			CanChooseFiles(false).
			CanCreateDirectories(true).
			AttachToWindow(window).
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
}

// installSaveFilePicker gives the desktop window a native save dialog, carrying
// the filters the request asked for.
func installSaveFilePicker(app *application.App, svc *gui.Service, window *application.WebviewWindow) {
	svc.SetSaveFilePicker(func(req gui.ChooseSaveFileRequestDTO) (string, error) {
		options := &application.SaveFileDialogOptions{
			Title:                req.Title,
			Directory:            req.DefaultDir,
			Filename:             req.DefaultFilename,
			CanCreateDirectories: true,
			Window:               window,
		}
		for _, f := range req.Filters {
			options.Filters = append(options.Filters, application.FileFilter{
				DisplayName: f.DisplayName,
				Pattern:     f.Pattern,
			})
		}
		dialog := app.Dialog.SaveFileWithOptions(options)
		chosen, err := dialog.PromptForSingleSelection()
		if err != nil {
			return "", nil
		}
		return chosen, nil
	})
}

// localRPGApplicationMenu is the native menu macOS shows in the global menu bar.
// It is the platform default minus the Help menu, whose only entry ("Learn More")
// opens wails.io rather than anything about LocalRPG; the application's own Help
// and About live in the frontend menu. The About item the AppMenu role adds
// reports the application name and description, so it already describes LocalRPG.
func localRPGApplicationMenu() *application.Menu {
	menu := application.NewMenu()
	if application.Get() != nil {
		menu.AddRole(application.AppMenu)
	}
	menu.AddRole(application.FileMenu)
	menu.AddRole(application.EditMenu)
	menu.AddRole(application.ViewMenu)
	menu.AddRole(application.WindowMenu)
	return menu
}
