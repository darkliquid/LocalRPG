# Settings Studio Restructure Design

**Date:** 2026-10-10
**Status:** Proposed
**Issue:** [#126](https://github.com/darkliquid/LocalRPG/issues/126)
**Epic:** [#122 Studio UX and authoring](https://github.com/darkliquid/LocalRPG/issues/122)
**Depends on:** [Provider Manager UI](2026-10-05-provider-manager-ui-design.md), [Provider Manager Completion](2026-10-06-provider-manager-completion-design.md), [Embedding Provider Manager UI](2026-10-07-embedding-provider-manager-ui-design.md)
**Scope:** `frontend`

---

## 1. Problem

`frontend/src/components/SettingsStudio.tsx` is 3,393 lines and owns eight top-level
tabs, two full provider editor forms (the TTS form alone is about 1,100 lines), and two
modals. It is the largest component in the repository. Three symptoms follow:

1. **Density.** Every tab is one flat column. "Providers" stacks cloud keys,
   embeddings, and three media provider lists; "AI Agents" stacks the role editor,
   twelve context/response limit fields, and two generation limit fields; "Media
   Engines" stacks three large provider editors. Nothing is progressively disclosed:
   the only collapsible control in the whole studio is a per-voice-profile `<details>`
   labelled "Provider Options" (`SettingsStudio.tsx:2760`).
2. **Duplicated media lists.** The Providers tab mounts `ProviderManager` for `tts`,
   `stt`, and `image` list-only (`SettingsStudio.tsx:645-647`), and the Media Engines
   tab mounts the same three families again with full editors (`:1901`, `:2796`,
   `:2993`). The same providers appear in two tabs, and only one of them is editable.
3. **Advanced options are always on screen.** `context_token_budget`, the recall and
   retrieval windows, `turn_timeout_seconds`, `tool_rounds`, and the generation limits
   are all visible in the AI Agents tab (`:1551-1894`), as are per-role `max_tokens`,
   `temperature`, `thinking_budget`, `top_p`, and `top_k`. Most users never set them.

## 2. Goals

- Each of the three configuration areas (Providers, AI Agents, Media Engines) gets a
  secondary tab bar that splits the settings currently stacked beneath it.
- Advanced options are hidden behind an explicit disclosure, collapsed by default.
- The duplicated media provider lists are removed from the Providers tab.
- The 3,393-line component is decomposed so each panel is a focused file.
- No change to what is persisted or to the config schema.

## 3. Non-goals

- Changing the config model, the save path, or the API. This is an information
  architecture and file-structure change.
- Redesigning `ProviderManager`'s list/edit behaviour, which stays as it is.
- Touching the Paths, Batch Jobs, Usage, or Debug tabs beyond extracting them.

## 4. Design

### 4.1 Information architecture

Keep the eight top-level tabs. Add a secondary tab bar inside the three dense ones.
The Providers tab loses its media lists (they were list-only duplicates of Media
Engines).

| Top tab | Secondary tabs | Owns |
| --- | --- | --- |
| Providers | Cloud Keys, Embeddings, Offline | shared account keys (Gemini, Inworld, Cartesia), the embeddings provider, the offline preset |
| AI Agents | Roles, Routing, Advanced | role to provider routing, presets/fallbacks, context/response/generation limits, timeouts, retrieval |
| Media Engines | TTS, STT, Images, Voices & Casting | the three media provider editors, the voice profile library, purposes/chains, playback preferences |

Paths, Batch Jobs, Preferences, Usage, and Debug keep their current single-column
layouts. Each secondary tab is deep-linkable through local component state only
(no routing change).

### 4.2 Advanced disclosure

Two mechanisms, both frontend-only:

- `components/ui/AdvancedSection.tsx`: a labelled disclosure (summary "Advanced",
  chevron, collapsed by default) that wraps a group of fields. The per-role Response
  Limit, Temperature, `thinking_budget`, `top_p`, and `top_k` move inside one in the
  role editor; the Context & Response Limits and Generation Limits cards become the
  body of the AI Agents **Advanced** secondary tab, so they are off the default path
  even without expanding anything else.
- A persistent `useAdvanced()` hook backed by `localStorage` key
  `localrpg.settings.advanced` (default `false`). When off, `AdvancedSection` renders
  collapsed and the AI Agents secondary bar shows Roles and Routing only. When on, the
  Advanced sub-tab appears and sections render expanded. A small "Show advanced
  settings" toggle in the studio header flips it.

A pure UI preference lives in `localStorage` rather than the config because it is not
machine configuration and does not need to sync or be saved server-side; the config
schema is left untouched. If a future release wants it portable, it becomes a
`preferences` field, and the hook is the single place to change.

### 4.3 File decomposition

`SettingsStudio.tsx` becomes a shell (tab bars, the draft `config`, `handleSave`, the
feedback banner, and the modals), targeting under 400 lines. Each panel moves to its
own file, taking `{ config, setConfig }` (and the props it already needs):

| New file | Extracted from |
| --- | --- |
| `components/settings/PathsPanel.tsx` | `:583-637` |
| `components/settings/ProvidersPanel.tsx` | `:640-956`, minus the three removed media managers |
| `components/settings/AgentsPanel.tsx` | `:959-1896` |
| `components/settings/MediaPanel.tsx` | `:1899-3230` |
| `components/settings/PreferencesPanel.tsx` | `:3238-3354` |
| `components/providers/CloudKeysPanel.tsx` | the Gemini/Inworld/Cartesia cards, `:715-956` |
| `components/providers/EmbeddingsPanel.tsx` | `:650-713` |
| `components/providers/TTSProviderEditor.tsx` | the TTS `renderEditor` body, `:1912-2790` |
| `components/providers/STTProviderEditor.tsx` | `:2807-2989` |
| `components/providers/ImageProviderEditor.tsx` | `:3004-3227` |
| `components/providers/LLMRoleEditor.tsx` | the per-role editor, `:1039-1547` |
| `components/settings/ContextLimitsPanel.tsx` | `:1551-1837` |
| `components/settings/GenerationLimitsPanel.tsx` | `:1839-1894` |
| `components/ui/SubTabs.tsx` | new, generic secondary tab bar |
| `components/ui/AdvancedSection.tsx` | new, generic disclosure |

`SubTabs` takes `{ tabs: { id, label, icon? }[], active, onChange }` and renders the
same button styling the top-level bar uses today. Media Engines' three provider
editors keep using `ProviderManager`'s `renderEditor` with the extracted components,
so the behaviour is unchanged.

## 5. Behaviour

| Action | Before | After |
| --- | --- | --- |
| Open Providers | cloud keys + embeddings + three media lists | Cloud Keys / Embeddings / Offline secondary tabs |
| Find the TTS editor | Media Engines, one long column | Media Engines, TTS secondary tab |
| Set `context_token_budget` | scroll the AI Agents column | AI Agents, Advanced (toggle on) |
| Set `max_tokens` for a role | always visible | inside the role editor's Advanced disclosure |
| Find where media providers are configured | two tabs show them | one tab |

## 6. Testing

- vitest: `SubTabs` switches and renders `aria-selected`; `AdvancedSection` is
  collapsed by default and expands; `useAdvanced` defaults to false and persists.
- vitest: the AI Agents panel hides the limit fields when advanced is off and shows
  them when on; the per-role editor hides `max_tokens`/`temperature` behind the
  disclosure.
- The existing `ProviderManager.test.tsx` and `SettingsStudio` tests are updated for
  the new structure and must stay green; `tsc --noEmit` and `npm run build` are the
  gate (unused imports fail the build, which matters during extraction).
- A manual pass: save settings after the restructure and confirm the persisted YAML is
  byte-identical to before the change for an unchanged form.

## 7. Rollout

Frontend only, no config or API change. Mechanical extraction first (files move, tests
stay green), then the secondary tab bars, then the advanced disclosure. Each step is
independently reviewable.

## 8. Risks

- **A mechanical extraction can silently change behaviour.** The Providers tab is the
  sharp edge: removing the three media `ProviderManager` mounts must not remove the
  only copy of anything. They are list-only (no `renderEditor`), and Media Engines owns
  the editors, so removal is safe; the extraction is verified by diffing the rendered
  tree for a fixture config against the pre-change build.
- **Deep state coupling.** All panels mutate one `config` draft through `setConfig`.
  Passing the same pair down keeps that contract; no panel gets its own copy, or two
  copies would diverge before save.
- **`localStorage` is per-origin.** The Wails webview and the browser-port origin are
  different origins, so an advanced toggle set in one is not seen in the other. That is
  acceptable for a UI preference and is stated in the toggle's tooltip.
- **Open question (confirm before implementation):** whether the TTS/STT/Image editors
  belong under Providers (as provider *instances*) or stay under Media Engines (as
  engines). This spec keeps them under Media Engines, because that is where users
  already find them, and removes only the redundant Providers lists. If the preferred
  model is "Providers owns every instance, Media Engines owns defaults and casting",
  the same `SubTabs`/extraction design applies with the editors relocated.
