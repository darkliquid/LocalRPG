package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/gui"
	"github.com/darkliquid/localrpg/pkg/paths"
	"github.com/darkliquid/localrpg/pkg/registry"
)

func handleRegistryCommand(args []string) {
	code := runRegistryCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

// registryProjectRoot is the directory a registry command treats as the project
// root: the current directory. The install path already targets it, because the
// content it writes belongs to the project the command runs in.
func registryProjectRoot() string {
	rootDir, _ := os.Getwd()
	return rootDir
}

// registryResolver resolves the content directories for a registry command. The
// configured paths override the defaults the way they do everywhere else, which
// is what keeps `registry search` from writing its index cache to a bare `cache/`
// folder beside wherever the command happened to run.
func registryResolver(cfg *config.Config) *core.PathResolver {
	dirs := paths.Resolve(paths.System(), cfg.Paths, registryProjectRoot())
	return core.NewCustomPathResolver(dirs.Systems, dirs.Worlds, dirs.Games, dirs.Cache)
}

func runRegistryCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printRegistryUsage(stderr)
		return 1
	}

	switch args[0] {
	case "add":
		return runRegistryAdd(args[1:], stdout, stderr)
	case "list":
		return runRegistryList(stdout, stderr)
	case "remove":
		return runRegistryRemove(args[1:], stdout, stderr)
	case "search":
		return runRegistrySearch(args[1:], stdout, stderr)
	case "install":
		return runRegistryInstall(args[1:], stdout, stderr)
	case "update":
		return runRegistryUpdate(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printRegistryUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown registry subcommand: %s\n", args[0])
		printRegistryUsage(stderr)
		return 1
	}
}

func printRegistryUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: localrpg registry <subcommand> [arguments]")
	fmt.Fprintln(w, "\nSubcommands:")
	fmt.Fprintln(w, "  add <url>                                               Add a package registry index URL")
	fmt.Fprintln(w, "  list                                                    List configured registries")
	fmt.Fprintln(w, "  remove <url>                                            Remove a registry")
	fmt.Fprintln(w, "  search [query]                                          Search for packages across registries")
	fmt.Fprintln(w, "  install <id>[@version] [--on-conflict mode] [--yes]     Install a package from a registry")
	fmt.Fprintln(w, "  update                                                  Check for package updates")
}

func runRegistryAdd(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg registry add <url>")
		return 1
	}
	rawURL, err := registry.NormalizeSource(args[0])
	if err != nil {
		fmt.Fprintf(stderr, "Invalid registry URL: %v\n", err)
		return 1
	}

	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	for _, u := range cfg.Registries.URLs {
		if u == rawURL {
			fmt.Fprintf(stdout, "Registry %q is already configured.\n", rawURL)
			return 0
		}
	}

	cfg.Registries.URLs = append(cfg.Registries.URLs, rawURL)
	if err := mgr.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "Save config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Added registry %q\n", rawURL)
	return 0
}

func runRegistryList(stdout, stderr io.Writer) int {
	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	if len(cfg.Registries.URLs) == 0 {
		fmt.Fprintln(stdout, "No registries configured.")
		return 0
	}

	fmt.Fprintln(stdout, "Configured Registries:")
	for _, u := range cfg.Registries.URLs {
		fmt.Fprintf(stdout, "  - %s\n", u)
	}
	return 0
}

func runRegistryRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg registry remove <url>")
		return 1
	}
	targetURL := strings.TrimSpace(args[0])

	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	var updated []string
	found := false
	for _, u := range cfg.Registries.URLs {
		if u == targetURL {
			found = true
			continue
		}
		updated = append(updated, u)
	}

	if !found {
		fmt.Fprintf(stderr, "Registry %q not found in configuration\n", targetURL)
		return 1
	}

	cfg.Registries.URLs = updated
	if err := mgr.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "Save config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Removed registry %q\n", targetURL)
	return 0
}

func runRegistrySearch(args []string, stdout, stderr io.Writer) int {
	query := ""
	if len(args) > 0 {
		query = args[0]
	}

	mgr := config.NewConfigManager()
	cfg, _ := mgr.Load()

	resolver := registryResolver(cfg)

	client := registry.NewClient(cfg.Registries, resolver.CacheDir())
	matches, err := client.Search(context.Background(), query)
	if err != nil {
		fmt.Fprintf(stderr, "Search error: %v\n", err)
		return 1
	}

	if len(matches) == 0 {
		fmt.Fprintln(stdout, "No matching packages found.")
		return 0
	}

	fmt.Fprintf(stdout, "%-8s %-20s %-10s %-20s %s\n", "TYPE", "ID", "VERSION", "REGISTRY", "NAME")
	for _, ref := range matches {
		fmt.Fprintf(stdout, "%-8s %-20s %-10s %-20s %s\n",
			ref.Package.Type,
			ref.Package.ID,
			ref.Package.Version,
			ref.RegistryName,
			ref.Package.Name,
		)
	}
	return 0
}

func runRegistryInstall(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("registry install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	onConflict := fs.String("on-conflict", "refuse", "Conflict resolution mode: refuse, rename, or overwrite")
	yes := fs.Bool("yes", false, "Confirm install without prompting")

	flags, positional := splitExportArgs(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 1
	}

	if len(positional) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg registry install <id>[@version] [--on-conflict refuse|rename|overwrite] [--yes]")
		return 1
	}

	spec := positional[0]
	var targetID, targetVersion string
	if parts := strings.SplitN(spec, "@", 2); len(parts) == 2 {
		targetID = parts[0]
		targetVersion = parts[1]
	} else {
		targetID = spec
	}

	mgr := config.NewConfigManager()
	cfg, _ := mgr.Load()

	resolver := registryResolver(cfg)

	client := registry.NewClient(cfg.Registries, resolver.CacheDir())
	matches, err := client.Search(context.Background(), targetID)
	if err != nil {
		fmt.Fprintf(stderr, "Search error: %v\n", err)
		return 1
	}

	var candidate *registry.PackageRef
	for i := range matches {
		ref := &matches[i]
		if ref.Package.ID == targetID {
			if targetVersion == "" || ref.Package.Version == targetVersion {
				candidate = ref
				break
			}
		}
	}

	if candidate == nil {
		if targetVersion != "" {
			fmt.Fprintf(stderr, "Package %s@%s not found in configured registries\n", targetID, targetVersion)
		} else {
			fmt.Fprintf(stderr, "Package %s not found in configured registries\n", targetID)
		}
		return 1
	}

	if !*yes {
		fmt.Fprintf(stderr, "Found %s %q (%s) from %s.\nRe-run with --yes to install.\n",
			candidate.Package.Type, candidate.Package.Name, candidate.Package.Version, candidate.RegistryName)
		return 1
	}

	svc := gui.NewService(registryProjectRoot())
	client.SetInstaller(func(ctx context.Context, r io.Reader, conflictMode string) (content.Manifest, error) {
		res, err := svc.ImportContent(ctx, r, conflictMode)
		if err != nil {
			return content.Manifest{}, err
		}
		return content.Manifest{
			ID:          res.ID,
			Name:        res.Name,
			Version:     res.Version,
			Type:        res.Type,
			Description: res.Description,
		}, nil
	})

	m, err := client.Install(context.Background(), *candidate, *onConflict)
	if err != nil {
		fmt.Fprintf(stderr, "Install failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Installed %s %q (%s) as %s\n", m.Type, m.Name, m.Version, m.ID)
	return 0
}

func runRegistryUpdate(args []string, stdout, stderr io.Writer) int {
	mgr := config.NewConfigManager()
	cfg, _ := mgr.Load()

	resolver := registryResolver(cfg)

	client := registry.NewClient(cfg.Registries, resolver.CacheDir())

	client.SetInstalledLister(func(ctx context.Context) ([]content.Manifest, error) {
		var manifests []content.Manifest

		// Scan worlds
		worldsDir := resolver.WorldsDir()
		if entries, err := os.ReadDir(worldsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					worldPath := filepath.Join(resolver.WorldDir(entry.Name()), "world.yaml")
					if wm, err := core.LoadWorldManifest(worldPath); err == nil && wm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      wm.ID,
							Name:    wm.Name,
							Version: wm.Version,
							Type:    "world",
						})
					}
				}
			}
		}

		// Scan systems
		systemsDir := resolver.SystemsDir()
		if entries, err := os.ReadDir(systemsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					sysPath := filepath.Join(resolver.SystemDir(entry.Name()), "system.yaml")
					if sm, err := core.LoadSystemManifest(sysPath); err == nil && sm != nil {
						manifests = append(manifests, content.Manifest{
							ID:      sm.ID,
							Name:    sm.Name,
							Version: sm.Version,
							Type:    "system",
						})
					}
				}
			}
		}

		return manifests, nil
	})

	updates, err := client.Update(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "Update check failed: %v\n", err)
		return 1
	}

	if len(updates) == 0 {
		fmt.Fprintln(stdout, "All installed content packages are up to date.")
		return 0
	}

	fmt.Fprintf(stdout, "%-8s %-20s %-10s %-20s %s\n", "TYPE", "ID", "NEW VERSION", "REGISTRY", "NAME")
	for _, ref := range updates {
		fmt.Fprintf(stdout, "%-8s %-20s %-10s %-20s %s\n",
			ref.Package.Type,
			ref.Package.ID,
			ref.Package.Version,
			ref.RegistryName,
			ref.Package.Name,
		)
	}
	return 0
}
