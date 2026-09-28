---
id: 07-storage-paths
title: Storage & Directories
category: Configuration & Providers
order: 7
description: Standard XDG directories, workspace mode, and disposable SQLite caching.
---

# Storage & Directories

LocalRPG follows modern operating system standards for file organization while supporting portable workspace modes.

## Standard XDG Storage Locations

By default, LocalRPG stores user data and caches in standard OS directories (via XDG specification):

### Linux & BSD

- **Data (`$XDG_DATA_HOME/localrpg` or `~/.local/share/localrpg/`)**:
  - `systems/`: Installed and custom game systems.
  - `worlds/`: Installed and custom worlds.
  - `games/`: Campaign folders, logs, and assets.
- **Cache (`$XDG_CACHE_HOME/localrpg` or `~/.cache/localrpg/`)**:
  - Downloaded models, generated audio cache, and image thumbnails.
- **Config (`$XDG_CONFIG_HOME/localrpg` or `~/.config/localrpg/config.yaml`)**:
  - Global user configuration and provider keys.

### macOS

- Data: `~/Library/Application Support/localrpg/`
- Cache: `~/Library/Caches/localrpg/`
- Config: `~/Library/Application Support/localrpg/config.yaml`

### Windows

- Data: `%LOCALAPPDATA%\localrpg\`
- Cache: `%LOCALAPPDATA%\localrpg\cache\`
- Config: `%APPDATA%\localrpg\config.yaml`

## Workspace & Project Mode

When you launch LocalRPG inside a repository containing `./localrpg.yaml` or pass the `--dir <path>` CLI flag:

- The working directory becomes the project root.
- Relative paths in configuration resolve directly against that root.
- Content is kept isolated, making campaigns and systems fully portable in Git repositories.

## Cache Safety & `cache/index.db`

Each campaign directory contains a SQLite database at `games/<id>/cache/index.db`.

- This database is strictly an index cache for rapid full-text search, graph queries, and turn listing.
- **It is 100% disposable**: If you delete `cache/index.db`, LocalRPG automatically rebuilds it from the Markdown notes and `history.jsonl` upon next launch.
- Never edit `index.db` directly; always edit the Markdown files in `entities/`.
