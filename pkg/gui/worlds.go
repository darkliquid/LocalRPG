// Worlds: their manifest, their entities, their own assets, and the tasks that name them.

package gui

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkliquid/localrpg/pkg/core"
	"github.com/darkliquid/localrpg/pkg/entity"
	"github.com/darkliquid/localrpg/pkg/media"
	"github.com/darkliquid/localrpg/pkg/pathutil"
	"gopkg.in/yaml.v3"
)

func (s *Service) ListWorlds(ctx context.Context) ([]WorldSummaryDTO, error) {
	worldsDir := s.resolver.WorldsDir()
	entries, err := os.ReadDir(worldsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []WorldSummaryDTO{}, nil
		}
		return nil, fmt.Errorf("read worlds dir: %w", err)
	}

	summaries := make([]WorldSummaryDTO, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifestPath := filepath.Join(worldsDir, e.Name(), "world.yaml")
		m, err := core.LoadWorldManifest(manifestPath)
		if err != nil {
			continue
		}
		compat := make([]string, 0)
		if m.DefaultSystem != "" {
			compat = append(compat, m.DefaultSystem)
		}
		var bannerURL, iconURL string
		worldDir := filepath.Join(worldsDir, e.Name())
		if p, _ := findAssetFile(worldDir, "banner"); p != "" {
			bannerURL = fmt.Sprintf("/api/world/%s/banner", m.ID)
		}
		if p, _ := findAssetFile(worldDir, "icon"); p != "" {
			iconURL = fmt.Sprintf("/api/world/%s/icon", m.ID)
		}

		summaries = append(summaries, WorldSummaryDTO{
			ID:                m.ID,
			Name:              m.Name,
			Description:       m.Description,
			Genre:             m.Genre,
			ArtStyle:          m.ArtStyle,
			Tags:              m.Tags,
			CompatibleSystems: compat,
			BannerURL:         bannerURL,
			IconURL:           iconURL,
		})
	}
	return summaries, nil
}

func (s *Service) GetWorld(ctx context.Context, id string) (*WorldDetailDTO, error) {
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid world id: %w", err)
	}

	worldDir := s.resolver.WorldDir(id)
	m, err := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load world manifest: %w", err)
	}

	entitiesDir := filepath.Join(worldDir, "entities")
	var entities []WorldEntitySummaryDTO
	_ = eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		// A world template is identified by its file name, unlike a campaign note:
		// templates are not indexed and are not linked by id, so taking the id from
		// the frontmatter here would rename every existing template the first time
		// it was saved.
		entID := strings.TrimSuffix(filepath.Base(path), ".md")
		name := entID
		entType := "concept"
		if ent, err := entity.ParseMarkdownEntity(data); err == nil {
			if ent.Name != "" {
				name = ent.Name
			}
			if ent.Type != "" {
				entType = ent.Type
			}
		}
		entities = append(entities, WorldEntitySummaryDTO{
			ID:     entID,
			Name:   name,
			Type:   entType,
			Folder: folder,
		})
		return nil
	})

	lorePrompt := ""
	if data, err := os.ReadFile(filepath.Join(worldDir, "prompts", "lore.md")); err == nil {
		lorePrompt = string(data)
	}

	return &WorldDetailDTO{
		ID:            m.ID,
		Name:          m.Name,
		Description:   m.Description,
		Genre:         m.Genre,
		DefaultSystem: m.DefaultSystem,
		ArtStyle:      m.ArtStyle,
		Tags:          m.Tags,
		LorePrompt:    lorePrompt,
		Entities:      entities,
	}, nil
}

// ErrWorldExists reports an attempt to create a world whose id is already taken.
var ErrWorldExists = errors.New("world already exists")

// ErrWorldNotFound reports an attempt to update a world that does not exist.
var ErrWorldNotFound = errors.New("world not found")

// ErrWorldInUse reports an attempt to delete a world that is still referenced by one or more campaigns.
var ErrWorldInUse = errors.New("world is in use by a campaign")

// writeWorld writes a world directory. It never decides create vs update; the
// caller does, so a create can refuse a duplicate and an update can require a
// target.
func (s *Service) writeWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if req.Name == "" {
		return nil, fmt.Errorf("world name is required")
	}
	id := req.ID
	if id == "" {
		id = slugify(req.Name)
	}
	if err := pathutil.ValidateID(id); err != nil {
		return nil, fmt.Errorf("invalid world id %q: %w", id, err)
	}

	worldDir := s.resolver.WorldDir(id)
	if err := os.MkdirAll(filepath.Join(worldDir, "entities"), 0755); err != nil {
		return nil, fmt.Errorf("create world entities dir: %w", err)
	}

	manifest := core.WorldManifest{
		ID:            id,
		Name:          req.Name,
		Description:   req.Description,
		Genre:         req.Genre,
		DefaultSystem: req.DefaultSystem,
		ArtStyle:      req.ArtStyle,
		Tags:          req.Tags,
	}
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshal world manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(worldDir, "world.yaml"), data, 0644); err != nil {
		return nil, fmt.Errorf("write world.yaml: %w", err)
	}

	lorePath := filepath.Join(worldDir, "prompts", "lore.md")
	if req.LorePrompt != "" {
		promptDir := filepath.Join(worldDir, "prompts")
		if err := os.MkdirAll(promptDir, 0755); err != nil {
			return nil, fmt.Errorf("create world prompts dir: %w", err)
		}
		if err := os.WriteFile(lorePath, []byte(req.LorePrompt), 0644); err != nil {
			return nil, fmt.Errorf("write lore.md: %w", err)
		}
	} else if err := os.Remove(lorePath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove lore.md: %w", err)
	}

	return s.GetWorld(ctx, id)
}

// CreateWorld writes a new world and refuses an id that is already taken.
func (s *Service) CreateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	id := ""
	if req.ID != "" {
		id = slugify(req.ID)
	}
	if id == "" {
		id = slugify(req.Name)
	}
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(id), "world.yaml")); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldExists, id)
	}
	req.ID = id
	return s.writeWorld(ctx, req)
}

// UpdateWorld rewrites an existing world and refuses an unknown id.
func (s *Service) UpdateWorld(ctx context.Context, req CreateWorldRequestDTO) (*WorldDetailDTO, error) {
	if _, err := os.Stat(filepath.Join(s.resolver.WorldDir(req.ID), "world.yaml")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrWorldNotFound, req.ID)
	}
	return s.writeWorld(ctx, req)
}

// DeleteWorld removes a world and all its entity templates, prompts, and assets.
// When force is false, it refuses to delete if any campaign references this world.
func (s *Service) DeleteWorld(ctx context.Context, worldID string, force bool) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}

	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(filepath.Join(worldDir, "world.yaml")); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrWorldNotFound, worldID)
		}
		return fmt.Errorf("world %q: %w", worldID, err)
	}

	if !force {
		games, err := s.ListGames(ctx)
		if err == nil {
			for _, g := range games {
				if g.WorldID == worldID {
					return fmt.Errorf("%w: campaign %q (%s)", ErrWorldInUse, g.Name, g.ID)
				}
			}
		}
	}

	if err := os.RemoveAll(worldDir); err != nil {
		return fmt.Errorf("remove world: %w", err)
	}
	return nil
}

func (s *Service) GetWorldEntity(ctx context.Context, worldID, entityID string) (*WorldEntityDetailDTO, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return nil, fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return nil, fmt.Errorf("invalid entity id: %w", err)
	}

	worldDir := s.resolver.WorldDir(worldID)
	path, err := findWorldEntityNote(worldDir, entityID)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("read world entity %s: %w", entityID, fs.ErrNotExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read world entity %s: %w", entityID, err)
	}
	folder := ""
	if rel, err := filepath.Rel(filepath.Join(worldDir, "entities"), filepath.Dir(path)); err == nil && rel != "." && rel != "" {
		folder = filepath.ToSlash(rel)
	}
	return &WorldEntityDetailDTO{
		ID:       entityID,
		Markdown: string(data),
		Folder:   folder,
	}, nil
}

// SaveWorldEntity writes a template into folder, moving it when it already lives
// somewhere else. World templates are not indexed, so there is no store to keep
// in step; the file is the record.
func (s *Service) SaveWorldEntity(ctx context.Context, worldID, entityID, folder, markdown string) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}
	cleanFolder, err := ValidateFolderPath(folder)
	if err != nil {
		return err
	}

	worldDir := s.resolver.WorldDir(worldID)
	entitiesDir := filepath.Join(worldDir, "entities")

	existingPath, err := findWorldEntityNote(worldDir, entityID)
	if err != nil {
		return err
	}

	targetDir := entitiesDir
	if cleanFolder != "" {
		targetDir = filepath.Join(entitiesDir, filepath.FromSlash(cleanFolder))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create world entities dir: %w", err)
	}

	targetPath, err := pathutil.ResolveSafeChild(targetDir, entityID+".md")
	if err != nil {
		return fmt.Errorf("resolve world entity path: %w", err)
	}
	if strings.Contains(targetPath, "..") {
		return fmt.Errorf("invalid world entity path: contains traversal")
	}
	if err := os.WriteFile(targetPath, []byte(markdown), 0o644); err != nil {
		return err
	}
	if existingPath != "" && existingPath != targetPath {
		if err := os.Remove(existingPath); err != nil {
			return fmt.Errorf("remove old template %q: %w", existingPath, err)
		}
	}
	return nil
}

func (s *Service) DeleteWorldEntity(ctx context.Context, worldID, entityID string) error {
	if err := pathutil.ValidateID(worldID); err != nil {
		return fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(entityID); err != nil {
		return fmt.Errorf("invalid entity id: %w", err)
	}

	path, err := findWorldEntityNote(s.resolver.WorldDir(worldID), entityID)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("delete world entity %s: %w", entityID, fs.ErrNotExist)
	}
	return os.Remove(path)
}

// findWorldEntityNote returns the path a world template occupies, or "" when it
// does not exist. A template is found by the id it declares or by its file name,
// so a hand-edited template that disagrees with itself is still reachable.
func findWorldEntityNote(worldDir, entityID string) (string, error) {
	entitiesDir := filepath.Join(worldDir, "entities")
	found := ""
	err := eachEntityNote(entitiesDir, func(path, folder string, data []byte) error {
		filenameID := strings.TrimSuffix(filepath.Base(path), ".md")
		declared := entity.DeclaredID(data)
		if filenameID == entityID || declared == entityID {
			found = path
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return found, nil
}

func (s *Service) GetWorldAsset(worldID, assetKind string) (string, string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", "", fmt.Errorf("invalid asset kind: %w", err)
	}
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(worldDir); err != nil {
		return "", "", os.ErrNotExist
	}
	p, ext := findAssetFile(worldDir, assetKind)
	if p == "" {
		return "", "", os.ErrNotExist
	}
	return p, contentTypeForArt(ext), nil
}

func (s *Service) SaveWorldAsset(worldID, assetKind string, data []byte, ext string) (string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(assetKind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	worldDir := s.resolver.WorldDir(worldID)
	if _, err := os.Stat(worldDir); err != nil {
		return "", fmt.Errorf("world not found: %w", err)
	}
	assetsDir := filepath.Join(worldDir, "assets")
	if err := os.MkdirAll(assetsDir, 0755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}
	if err := removeAssetFiles(assetsDir, assetKind); err != nil {
		return "", err
	}
	if ext == "" {
		ext = ".png"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	targetPath, err := pathutil.ResolveSafeChild(assetsDir, assetKind+ext)
	if err != nil {
		return "", fmt.Errorf("invalid asset path: %w", err)
	}
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return "", fmt.Errorf("write asset: %w", err)
	}
	return fmt.Sprintf("/api/world/%s/%s", worldID, assetKind), nil
}

func (s *Service) GenerateWorldAsset(ctx context.Context, worldID string, req GenerateAssetRequestDTO) (string, error) {
	if err := pathutil.ValidateID(worldID); err != nil {
		return "", fmt.Errorf("invalid world id: %w", err)
	}
	if err := pathutil.ValidateID(req.Kind); err != nil {
		return "", fmt.Errorf("invalid asset kind: %w", err)
	}
	prompt := req.Prompt
	if prompt == "" {
		worldDir := s.resolver.WorldDir(worldID)
		wm, _ := core.LoadWorldManifest(filepath.Join(worldDir, "world.yaml"))
		worldName := worldID
		artStyle := ""
		genre := ""
		desc := ""
		if wm != nil {
			if wm.Name != "" {
				worldName = wm.Name
			}
			artStyle = wm.ArtStyle
			genre = wm.Genre
			desc = wm.Description
		}
		prompt = buildAssetPrompt(req.Kind, worldName, desc, artStyle, genre)
	}
	imgBytes, failure := s.generateImage(ctx, req.Kind, prompt, usageScope{})
	if failure != nil {
		return "", failure
	}
	return s.SaveWorldAsset(worldID, req.Kind, imgBytes, media.ArtExtension(imgBytes))
}
