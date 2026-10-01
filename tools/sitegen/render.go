package main

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// navItem is one entry in the launcher-style dock.
type navItem struct {
	Key      string
	Label    string
	Icon     string
	Href     string
	External bool
}

// pageData is everything a page template can render. A single struct keeps the
// dock and footer identical across pages without threading two data types.
type pageData struct {
	Root        string // "" at the site root, "../" from inside docs/
	Title       string
	Description string
	Active      string
	IsHome      bool
	Nav         []navItem
	Groups      []docGroup
	Doc         *doc
	Content     template.HTML
	Shots       []shot
	ShotCount   int
	RepoURL     string
	Version     string
	Year        int
}

// linkIndex maps documentation to its published URLs so cross-references
// inside the guide keep working once it is HTML.
type linkIndex struct {
	byID     map[string]string
	bySource map[string]string
	repoURL  string
}

func newLinkIndex(docs []doc, repoURL string) linkIndex {
	ix := linkIndex{
		byID:     map[string]string{},
		bySource: map[string]string{},
		repoURL:  strings.TrimRight(repoURL, "/"),
	}
	for _, d := range docs {
		ix.byID[d.ID] = d.URL
		ix.bySource[d.Source] = d.URL
		ix.bySource[strings.TrimSuffix(d.Source, ".md")] = d.URL
	}
	return ix
}

// resolve turns a link written for the repository into a link that works on
// the published site, falling back to the repository for anything the site
// does not carry.
func (ix linkIndex) resolve(current doc, target string) string {
	if target == "" || strings.HasPrefix(target, "#") || hasScheme(target) {
		return target
	}

	base, fragment := splitFragment(target)
	clean := strings.TrimPrefix(strings.TrimPrefix(base, "./"), "/")

	candidates := make([]string, 0, 2)
	if !strings.HasPrefix(base, "/") {
		candidates = append(candidates, path.Join(path.Dir(current.Source), clean))
	}
	candidates = append(candidates, clean)

	// Indexed URLs are written from the site root, but a link inside an article
	// is resolved against that article's own directory, so it needs the same
	// climb back to the root that the page templates use.
	prefix := rootPrefix(current.URL)
	for _, candidate := range candidates {
		if url, ok := ix.bySource[candidate]; ok {
			return prefix + url + fragment
		}
	}
	if url, ok := ix.byID[strings.TrimSuffix(path.Base(clean), ".md")]; ok {
		return prefix + url + fragment
	}
	return ix.repoURL + "/blob/main/" + candidates[0] + fragment
}

// rootPrefix is the relative path from a page's directory back to the site
// root, e.g. "../" for "docs/foo.html" and "" for "index.html".
func rootPrefix(url string) string {
	dir := path.Dir(url)
	if dir == "." || dir == "/" {
		return ""
	}
	depth := len(strings.Split(strings.Trim(dir, "/"), "/"))
	return strings.Repeat("../", depth)
}

func hasScheme(target string) bool {
	return strings.HasPrefix(target, "//") ||
		strings.Contains(target, "://") ||
		strings.HasPrefix(target, "mailto:") ||
		strings.HasPrefix(target, "tel:")
}

func splitFragment(target string) (string, string) {
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		return target[:i], target[i:]
	}
	return target, ""
}

// linkTransformer rewrites link destinations as the Markdown is parsed, which
// is cheaper and safer than patching the rendered HTML afterwards.
type linkTransformer struct {
	resolve func(string) string
}

func (t *linkTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := node.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		link.Destination = []byte(t.resolve(string(link.Destination)))
		return ast.WalkContinue, nil
	})
}

// renderMarkdown converts one article to HTML, rewriting its links on the way.
func renderMarkdown(body string, ix linkIndex, current doc) (template.HTML, error) {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(util.Prioritized(&linkTransformer{
				resolve: func(target string) string { return ix.resolve(current, target) },
			}, 100)),
		),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return "", fmt.Errorf("render %s: %w", current.Source, err)
	}
	return template.HTML(buf.String()), nil
}

// renderer turns a page template plus shared data into HTML.
type renderer struct {
	base  string
	funcs template.FuncMap
}

func newRenderer() (*renderer, error) {
	base, err := fs.ReadFile(templateFS, "templates/base.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("read base template: %w", err)
	}
	return &renderer{base: string(base), funcs: template.FuncMap{"icon": icon}}, nil
}

func (r *renderer) render(pageFile string, data *pageData) ([]byte, error) {
	page, err := fs.ReadFile(templateFS, "templates/"+pageFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", pageFile, err)
	}

	tmpl, err := template.New("base").Funcs(r.funcs).Parse(r.base)
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}
	if _, err := tmpl.Parse(string(page)); err != nil {
		return nil, fmt.Errorf("parse %s: %w", pageFile, err)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		return nil, fmt.Errorf("render %s: %w", pageFile, err)
	}
	return buf.Bytes(), nil
}

// build renders the whole site into cfg.out.
func build(cfg config) error {
	if err := guardOutput(cfg.out); err != nil {
		return err
	}

	docs, err := loadDocs(cfg)
	if err != nil {
		return err
	}
	ix := newLinkIndex(docs, cfg.repoURL)
	groups := groupDocs(docs)
	shots := loadShots(cfg.screenshots)
	version := readVersion(cfg.root)
	year := time.Now().Year()

	r, err := newRenderer()
	if err != nil {
		return err
	}

	if err := os.RemoveAll(cfg.out); err != nil {
		return fmt.Errorf("clear output directory: %w", err)
	}
	if err := os.MkdirAll(cfg.out, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := copyEmbeddedAssets(cfg.out); err != nil {
		return err
	}
	if err := copyShots(cfg.screenshots, filepath.Join(cfg.out, "screenshots"), shots); err != nil {
		return err
	}

	common := func(root, title, description, active string) *pageData {
		return &pageData{
			Root:        root,
			Title:       title,
			Description: description,
			Active:      active,
			Nav:         nav(root, cfg.repoURL),
			Groups:      groups,
			Shots:       shots,
			ShotCount:   presentShots(shots),
			RepoURL:     strings.TrimRight(cfg.repoURL, "/"),
			Version:     version,
			Year:        year,
		}
	}

	home := common("", "LocalRPG", "A local-first, turn-based tabletop RPG client with a glassmorphic desktop launcher, terminal client and story theatre.", "home")
	home.IsHome = true
	if err := writePage(r, cfg.out, "index.html", "home.html.tmpl", home); err != nil {
		return err
	}

	index := common("../", "Documentation", "Guides for worlds, systems, campaigns, providers and the studios.", "docs")
	if err := writePage(r, cfg.out, filepath.Join("docs", "index.html"), "docs.html.tmpl", index); err != nil {
		return err
	}

	for _, article := range docs {
		content, err := renderMarkdown(article.Body, ix, article)
		if err != nil {
			return err
		}
		page := common("../", article.Title, article.Description, "docs")
		page.Doc = &article
		page.Content = content
		if err := writePage(r, cfg.out, filepath.Join("docs", article.ID+".html"), "doc.html.tmpl", page); err != nil {
			return err
		}
	}

	// Pages serves files verbatim when this marker is present; without it the
	// Jekyll pipeline would try to process the assets.
	if err := os.WriteFile(filepath.Join(cfg.out, ".nojekyll"), nil, 0o644); err != nil {
		return fmt.Errorf("write .nojekyll: %w", err)
	}

	fmt.Printf("sitegen: wrote %d pages and %d/%d screenshots to %s\n",
		len(docs)+2, presentShots(shots), len(shots), cfg.out)
	return nil
}

func writePage(r *renderer, out, rel, templateFile string, data *pageData) error {
	body, err := r.render(templateFile, data)
	if err != nil {
		return err
	}
	target := filepath.Join(out, rel)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}

func copyEmbeddedAssets(out string) error {
	assets, err := fs.Sub(assetFS, "assets")
	if err != nil {
		return fmt.Errorf("locate assets: %w", err)
	}
	if err := os.CopyFS(filepath.Join(out, "assets"), assets); err != nil {
		return fmt.Errorf("write assets: %w", err)
	}
	return nil
}

// copyShots publishes only the gallery images that exist, so a stray note in
// the screenshots directory never ends up on the site.
func copyShots(srcDir, dstDir string, shots []shot) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("create screenshots directory: %w", err)
	}
	for _, s := range shots {
		if !s.Present {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, s.File))
		if err != nil {
			return fmt.Errorf("read screenshot %s: %w", s.File, err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, s.File), data, 0o644); err != nil {
			return fmt.Errorf("write screenshot %s: %w", s.File, err)
		}
	}
	return nil
}

// guardOutput refuses to delete anything that is not clearly a build directory.
func guardOutput(out string) error {
	cleaned := filepath.Clean(out)
	if cleaned == "." || cleaned == string(filepath.Separator) || !strings.Contains(cleaned, string(filepath.Separator)) {
		return fmt.Errorf("refusing to clear %q: point -out at a build directory", out)
	}
	return nil
}

var versionPattern = regexp.MustCompile(`(?m)^var Version = "([^"]+)"`)

// readVersion lifts the reported version out of the CLI's main package so the
// site advertises the same number the binary prints.
func readVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "cmd", "localrpg", "main.go"))
	if err != nil {
		return ""
	}
	match := versionPattern.FindSubmatch(data)
	if match == nil {
		return ""
	}
	return string(match[1])
}

// nav builds the dock for a page, using root so links work from any depth.
func nav(root, repoURL string) []navItem {
	repoURL = strings.TrimRight(repoURL, "/")
	home := root + "index.html"
	return []navItem{
		{Key: "home", Label: "Overview", Icon: "home", Href: home},
		{Key: "features", Label: "Features", Icon: "grid", Href: home + "#features"},
		{Key: "screenshots", Label: "Screenshots", Icon: "image", Href: home + "#screenshots"},
		{Key: "docs", Label: "Documentation", Icon: "book", Href: root + "docs/index.html"},
		{Key: "download", Label: "Download", Icon: "download", Href: repoURL + "/releases", External: true},
		{Key: "github", Label: "GitHub", Icon: "github", Href: repoURL, External: true},
	}
}

type iconSpec struct {
	body string
	fill bool
}

// icons are the inline SVGs the dock and feature cards use. They are kept in
// code so the site ships no icon font and no network request.
var icons = map[string]iconSpec{
	"home": {
		body: `<path d="M3 10.5 12 3l9 7.5"/><path d="M5.5 9.5V21h13V9.5"/><path d="M9.75 21v-5.5h4.5V21"/>`,
	},
	"grid": {
		body: `<rect x="3" y="3" width="7.5" height="7.5" rx="2"/><rect x="13.5" y="3" width="7.5" height="7.5" rx="2"/><rect x="3" y="13.5" width="7.5" height="7.5" rx="2"/><rect x="13.5" y="13.5" width="7.5" height="7.5" rx="2"/>`,
	},
	"image": {
		body: `<rect x="3" y="3" width="18" height="18" rx="3"/><circle cx="8.75" cy="8.75" r="1.6"/><path d="m20.5 15.5-4.9-4.9L5 21"/>`,
	},
	"book": {
		body: `<path d="M12 7v14"/><path d="M4 4.5A1.5 1.5 0 0 1 5.5 3H10a2 2 0 0 1 2 2v16a2 2 0 0 0-2-2H4z"/><path d="M20 4.5A1.5 1.5 0 0 0 18.5 3H14a2 2 0 0 0-2 2v16a2 2 0 0 1 2-2h6z"/>`,
	},
	"download": {
		body: `<path d="M12 3.5v11"/><path d="m7.5 10.5 4.5 4.5 4.5-4.5"/><path d="M4 20.5h16"/>`,
	},
	"sparkles": {
		body: `<path d="m12 3 1.7 4.8L18.5 9.5l-4.8 1.7L12 16l-1.7-4.8L5.5 9.5l4.8-1.7z"/><path d="M18.5 15.5l.8 2.2 2.2.8-2.2.8-.8 2.2-.8-2.2-2.2-.8 2.2-.8z"/>`,
	},
	"shield": {
		body: `<path d="M12 3 5 6v6c0 4.2 2.9 7.6 7 9 4.1-1.4 7-4.8 7-9V6z"/><path d="m9.2 12.2 2 2 3.6-3.9"/>`,
	},
	"cpu": {
		body: `<rect x="6.5" y="6.5" width="11" height="11" rx="2.5"/><path d="M10 3v3.5M14 3v3.5M10 17.5V21M14 17.5V21M3 10h3.5M3 14h3.5M17.5 10H21M17.5 14H21"/>`,
	},
	"terminal": {
		body: `<rect x="2.5" y="4" width="19" height="16" rx="3"/><path d="m7 9.5 2.5 2.5L7 14.5"/><path d="M12.5 15h4"/>`,
	},
	"layers": {
		body: `<path d="m12 3 8.5 4.5L12 12 3.5 7.5z"/><path d="m4 12 8 4.5 8-4.5"/><path d="m4 16.5 8 4.5 8-4.5"/>`,
	},
	"github": {
		fill: true,
		body: `<path d="M12 .5A11.5 11.5 0 0 0 .5 12a11.5 11.5 0 0 0 7.86 10.92c.58.1.79-.25.79-.56v-2c-3.2.7-3.88-1.37-3.88-1.37-.53-1.34-1.29-1.7-1.29-1.7-1.05-.72.08-.7.08-.7 1.16.08 1.77 1.19 1.77 1.19 1.03 1.77 2.7 1.26 3.36.96.1-.75.4-1.26.73-1.55-2.55-.29-5.24-1.28-5.24-5.7 0-1.26.45-2.29 1.19-3.1-.12-.29-.52-1.46.11-3.05 0 0 .97-.31 3.18 1.18a11 11 0 0 1 5.79 0c2.2-1.49 3.17-1.18 3.17-1.18.63 1.59.23 2.76.11 3.05.74.81 1.19 1.84 1.19 3.1 0 4.43-2.7 5.4-5.26 5.69.42.36.79 1.07.79 2.15v3.19c0 .31.21.67.8.56A11.5 11.5 0 0 0 23.5 12 11.5 11.5 0 0 0 12 .5z"/>`,
	},
}

// icon renders one inline SVG by name.
func icon(name string) template.HTML {
	spec, ok := icons[name]
	if !ok {
		return ""
	}
	attrs := `viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"`
	if spec.fill {
		attrs = `viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"`
	}
	return template.HTML(`<svg class="icon" ` + attrs + `>` + spec.body + `</svg>`)
}
