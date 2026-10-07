package main

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/darkliquid/localrpg/pkg/config"
	"github.com/darkliquid/localrpg/pkg/content"
)

func handlePublisherCommand(args []string) {
	code := runPublisherCommand(args, os.Stdout, os.Stderr)
	if code != 0 {
		os.Exit(code)
	}
}

func runPublisherCommand(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printPublisherUsage(stderr)
		return 1
	}

	switch args[0] {
	case "add":
		return runPublisherAdd(args[1:], stdout, stderr)
	case "list":
		return runPublisherList(stdout, stderr)
	case "remove":
		return runPublisherRemove(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printPublisherUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "Unknown publisher subcommand: %s\n", args[0])
		printPublisherUsage(stderr)
		return 1
	}
}

func printPublisherUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage: localrpg publisher <subcommand> [arguments]")
	fmt.Fprintln(w, "\nSubcommands:")
	fmt.Fprintln(w, "  add <key-file|fingerprint> --name <name>  Add a trusted publisher")
	fmt.Fprintln(w, "  list                                      List all trusted publishers")
	fmt.Fprintln(w, "  remove <fingerprint>                      Remove a trusted publisher")
}

func runPublisherAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("publisher add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	name := fs.String("name", "", "Publisher display name")

	flags, positional := splitExportArgs(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 1
	}

	if len(positional) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg publisher add <pubkey-file|fingerprint> --name <name>")
		return 1
	}

	target := positional[0]
	fp, err := loadPublicKeyOrFingerprint(target)
	if err != nil {
		fmt.Fprintf(stderr, "Load public key or fingerprint: %v\n", err)
		return 1
	}

	displayName := *name
	if displayName == "" {
		displayName = fp[:12]
	}

	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	if cfg.Publishers == nil {
		cfg.Publishers = make(map[string]string)
	}
	cfg.Publishers[fp] = displayName

	if err := mgr.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "Save config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Added publisher %q (%s)\n", displayName, fp)
	return 0
}

func runPublisherList(stdout, stderr io.Writer) int {
	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	if len(cfg.Publishers) == 0 {
		fmt.Fprintln(stdout, "No trusted publishers configured.")
		return 0
	}

	keys := make([]string, 0, len(cfg.Publishers))
	for k := range cfg.Publishers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Fprintf(stdout, "%-64s  %s\n", "FINGERPRINT", "NAME")
	for _, fp := range keys {
		fmt.Fprintf(stdout, "%-64s  %s\n", fp, cfg.Publishers[fp])
	}
	return 0
}

func runPublisherRemove(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg publisher remove <fingerprint>")
		return 1
	}

	fp := strings.ToLower(strings.TrimSpace(args[0]))
	mgr := config.NewConfigManager()
	cfg, err := mgr.Load()
	if err != nil {
		fmt.Fprintf(stderr, "Load config: %v\n", err)
		return 1
	}

	if cfg.Publishers == nil || cfg.Publishers[fp] == "" {
		fmt.Fprintf(stderr, "Publisher with fingerprint %s not found\n", fp)
		return 1
	}

	delete(cfg.Publishers, fp)
	if err := mgr.Save(cfg); err != nil {
		fmt.Fprintf(stderr, "Save config: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "Removed publisher %s\n", fp)
	return 0
}

func runContentSign(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("content sign", flag.ContinueOnError)
	fs.SetOutput(stderr)
	keyPath := fs.String("key", "", "Path to private key file")
	publisher := fs.String("publisher", "", "Publisher display name")

	flags, positional := splitExportArgs(fs, args)
	if err := fs.Parse(flags); err != nil {
		return 1
	}

	if len(positional) < 1 || *keyPath == "" {
		fmt.Fprintln(stderr, "Usage: localrpg content sign <package|directory> --key <privkey-file> [--publisher <name>]")
		return 1
	}

	target := positional[0]
	privKey, err := loadPrivateKey(*keyPath)
	if err != nil {
		fmt.Fprintf(stderr, "Load private key: %v\n", err)
		return 1
	}

	fi, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(stderr, "Stat target %q: %v\n", target, err)
		return 1
	}

	if fi.IsDir() {
		if err := content.SignDirectory(target, privKey, *publisher); err != nil {
			fmt.Fprintf(stderr, "Sign directory: %v\n", err)
			return 1
		}
	} else {
		if err := content.SignPackageFile(target, privKey, *publisher); err != nil {
			fmt.Fprintf(stderr, "Sign package: %v\n", err)
			return 1
		}
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	fp := content.Fingerprint(pubKey)
	fmt.Fprintf(stdout, "Signed %s (key: %s, publisher: %s)\n", target, fp, *publisher)
	return 0
}

func runContentVerify(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "Usage: localrpg content verify <package|directory>")
		return 1
	}

	target := args[0]
	m, sigBytes, err := content.ReadPackageManifestAndSig(target)
	if err != nil {
		fmt.Fprintf(stderr, "Error verifying %s: invalid: %v\n", target, err)
		return 1
	}

	mgr := config.NewConfigManager()
	cfg, _ := mgr.Load()

	var trusted map[string]string
	if cfg != nil {
		trusted = cfg.Publishers
	}

	trust, verifyErr := content.Verify(m, sigBytes, trusted)
	if verifyErr != nil || trust.State == "invalid" {
		fmt.Fprintf(stderr, "Package signature is invalid: %v\n", verifyErr)
		return 1
	}

	switch trust.State {
	case "verified":
		fmt.Fprintf(stdout, "Package verified (publisher: %s, key: %s)\n", trust.Publisher, trust.Fingerprint)
		return 0
	case "unknown_key":
		fmt.Fprintf(stdout, "Package signed by unknown_key (publisher: %s, key: %s)\n", trust.Publisher, trust.Fingerprint)
		return 0
	case "unsigned":
		fmt.Fprintln(stdout, "Package is unsigned")
		return 0
	default:
		fmt.Fprintf(stderr, "Package signature is invalid: state %s\n", trust.State)
		return 1
	}
}

func loadPrivateKey(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key file: %w", err)
	}

	str := strings.TrimSpace(string(data))

	// Check for PEM block
	if block, _ := pem.Decode(data); block != nil {
		pkcs8Key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			if k, ok := pkcs8Key.(ed25519.PrivateKey); ok {
				return k, nil
			}
		}
		if len(block.Bytes) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(block.Bytes), nil
		}
		if len(block.Bytes) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(block.Bytes), nil
		}
	}

	// Try hex string (64 chars for 32-byte seed, or 128 chars for 64-byte private key)
	if b, err := hex.DecodeString(str); err == nil {
		if len(b) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(b), nil
		}
		if len(b) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(b), nil
		}
	}

	// Try base64
	if b, err := base64.StdEncoding.DecodeString(str); err == nil {
		if len(b) == ed25519.SeedSize {
			return ed25519.NewKeyFromSeed(b), nil
		}
		if len(b) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(b), nil
		}
	}

	// Raw binary
	if len(data) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(data), nil
	}
	if len(data) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(data), nil
	}

	return nil, fmt.Errorf("invalid private key in %q: expected 32-byte seed or 64-byte Ed25519 key", path)
}

func loadPublicKeyOrFingerprint(pathOrKey string) (string, error) {
	if data, err := os.ReadFile(pathOrKey); err == nil {
		str := strings.TrimSpace(string(data))

		// Check for PEM
		if block, _ := pem.Decode(data); block != nil {
			if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
				if k, ok := pub.(ed25519.PublicKey); ok {
					return content.Fingerprint(k), nil
				}
			}
			if len(block.Bytes) == ed25519.PublicKeySize {
				return content.Fingerprint(ed25519.PublicKey(block.Bytes)), nil
			}
		}

		// Try hex
		if b, err := hex.DecodeString(str); err == nil {
			if len(b) == ed25519.PublicKeySize {
				return content.Fingerprint(ed25519.PublicKey(b)), nil
			}
		}

		// Try base64
		if b, err := base64.StdEncoding.DecodeString(str); err == nil {
			if len(b) == ed25519.PublicKeySize {
				return content.Fingerprint(ed25519.PublicKey(b)), nil
			}
		}

		// Raw 32 bytes binary
		if len(data) == ed25519.PublicKeySize {
			return content.Fingerprint(ed25519.PublicKey(data)), nil
		}
	}

	// Direct 64-char hex string
	trimmed := strings.ToLower(strings.TrimSpace(pathOrKey))
	if len(trimmed) == 64 {
		if _, err := hex.DecodeString(trimmed); err == nil {
			return trimmed, nil
		}
	}

	// Direct base64 string of public key
	if b, err := base64.StdEncoding.DecodeString(pathOrKey); err == nil && len(b) == ed25519.PublicKeySize {
		return content.Fingerprint(ed25519.PublicKey(b)), nil
	}

	return "", fmt.Errorf("unable to resolve public key or fingerprint from %q", pathOrKey)
}
