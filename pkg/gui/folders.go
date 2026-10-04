package gui

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrInvalidFolderPath reports a folder path a client sent that could escape the
// entities directory or name something the walk would then skip.
var ErrInvalidFolderPath = errors.New("invalid folder path")

// maxFolderSegment bounds one directory name, so a path cannot be used to create
// a name no filesystem will accept.
const maxFolderSegment = 64

// ValidateFolderPath normalises and checks a folder path supplied by a client and
// returns its canonical form: forward slashes, no leading or trailing slash, and
// "" for the root. Nothing reaches the filesystem until this has passed.
func ValidateFolderPath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "/" {
		return "", nil
	}
	if strings.HasPrefix(trimmed, "/") || strings.Contains(trimmed, "\\") || strings.ContainsRune(trimmed, 0) {
		return "", fmt.Errorf("%w: %q", ErrInvalidFolderPath, raw)
	}
	if strings.HasSuffix(trimmed, "/") {
		return "", fmt.Errorf("%w: trailing slash in %q", ErrInvalidFolderPath, raw)
	}

	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		switch {
		case segment == "":
			return "", fmt.Errorf("%w: empty segment in %q", ErrInvalidFolderPath, raw)
		case segment == "." || segment == "..":
			return "", fmt.Errorf("%w: relative segment %q", ErrInvalidFolderPath, segment)
		case strings.HasPrefix(segment, "."):
			return "", fmt.Errorf("%w: hidden segment %q", ErrInvalidFolderPath, segment)
		case len(segment) > maxFolderSegment:
			return "", fmt.Errorf("%w: segment longer than %d characters", ErrInvalidFolderPath, maxFolderSegment)
		}
	}
	return strings.Join(segments, "/"), nil
}

// FolderDTO is one directory in a collection's entities tree. Folders are pure
// directories: the name is the label and there is no metadata file.
type FolderDTO struct {
	Path     string      `json:"path"`
	Name     string      `json:"name"`
	Children []FolderDTO `json:"children,omitempty"`
}

// BuildFolderTree turns a flat list of folder paths into a sorted tree. Every
// ancestor of a listed path is present, so an empty parent is never hidden by a
// child that exists.
func BuildFolderTree(paths []string) []FolderDTO {
	type node struct {
		name     string
		path     string
		children map[string]*node
	}

	root := &node{children: map[string]*node{}}
	for _, path := range paths {
		if path == "" {
			continue
		}
		current := root
		prefix := ""
		for _, segment := range strings.Split(path, "/") {
			if prefix == "" {
				prefix = segment
			} else {
				prefix = prefix + "/" + segment
			}
			child, ok := current.children[segment]
			if !ok {
				child = &node{name: segment, path: prefix, children: map[string]*node{}}
				current.children[segment] = child
			}
			current = child
		}
	}

	var build func(*node) []FolderDTO
	build = func(n *node) []FolderDTO {
		names := make([]string, 0, len(n.children))
		for name := range n.children {
			names = append(names, name)
		}
		sort.Strings(names)

		out := make([]FolderDTO, 0, len(names))
		for _, name := range names {
			child := n.children[name]
			out = append(out, FolderDTO{
				Path:     child.path,
				Name:     child.name,
				Children: build(child),
			})
		}
		return out
	}
	return build(root)
}
