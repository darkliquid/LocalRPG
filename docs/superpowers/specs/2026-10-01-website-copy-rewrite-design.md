# Website & README Copy Rewrite Design

## Goal
Refocus the marketing and project copy on what makes LocalRPG fit for purpose: creating original worlds, designing tabletop rules, and playing immersive AI campaigns. Remove technical implementation details (zero-TCP, YAML, TUI, 3-tier memory, single binary).

## Key Pillars Highlighted
1. **First-Class Local AI:** First-class support for local LLMs, TTS, and image generation (Ollama, LM Studio, ComfyUI, Piper, etc.) plus out-of-the-box built-in zero-GPU storytelling.
2. **TTS Narration & NPC Speech:** Full vocal narration of scene prose and individual voice profiles for NPCs and companions.
3. **Generated Scene Imagery:** Dynamically generated location art and character portraits reflecting world aesthetic.
4. **Custom System Mechanics:** Design custom tabletop rules, dice mechanics, and checks in the Systems Studio without hardcoded stats.
5. **Living Worlds & Campaigns:** Craft lore, factions, and starter content in Worlds Studio; track emerging relationships and discovered entities in an evolving campaign Codex.
6. **Story Theatre & Export:** Replay campaigns in a visual novel theatre mode, and export them as standalone web packages or rendered videos.

## Removed Concepts
- Zero-TCP execution / 0600 socket daemon details
- YAML frontmatter / manifest details
- Terminal TUI / Bubbletea client
- "3 tiers of on-disk memory"
- "One binary for GUI, TUI and API"

## Files to Modify
- `tools/sitegen/templates/home.html.tmpl`: Hero eyebrow, lede, facts, feature cards, quickstart.
- `tools/sitegen/templates/base.html.tmpl`: Footer subtitle.
- `tools/sitegen/render.go`: Meta description.
- `README.md`: Overview, feature highlights, gameplay & engine overview, media/export sections.
