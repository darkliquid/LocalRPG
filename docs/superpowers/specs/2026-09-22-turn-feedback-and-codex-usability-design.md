# Design Spec: Turn Feedback, Note Corrections, and Codex Usability

Date: 2026-09-22
Status: Approved

## 1. Overview & Goals

This specification resolves three critical usability issues in LocalRPG's desktop client:
1. **Turn-in-Progress Feedback**: While a player action is being processed by the backend and model provider, the interface currently lacks clear progress feedback, giving the impression that the app has locked up or frozen.
2. **Non-Disruptive Note Creation for Continuity Findings**: When addressing a continuity finding about an unknown entity or missing note, the client previously submitted a `/gm` turn to the LLM. This erroneously advanced the chronicle, generated unnecessary story narration, and interrupted active audio speech playback.
3. **Codex Drawer Sizing and Layout**: The drawer component (`max-w-xl` / 576px) squeezed the side-by-side entity list and note editor into cramped viewports, causing awkward horizontal scrolling. The drawer needs expanded default width, a full-screen maximize toggle, and a collapsible-by-default entity sidebar.

---

## 2. Detailed Architecture & Design

### 2.1 Inline Turn-in-Progress Indicator

#### Current State
- When submitting an action via `ActionConsole`:
  - `turnInFlight` becomes `true`.
  - Console inputs disable and submit becomes a "STOP" button.
  - The chronicle area (`ChronicleView`) does not change until the first streaming text chunk arrives (`streamedProse`).
  - For models with latency (or non-streaming providers), the chronicle remains static for several seconds, leaving the user unsure if anything is happening.

#### Target Behavior
- In `frontend/src/App.tsx`:
  - Track `pendingAction: { mode: string; text: string } | null` alongside `turnInFlight`.
  - On action submission, immediately set `pendingAction = { mode, text }`.
  - Pass `pendingAction`, `turnInFlight`, and `streamedProse` into `ChronicleView`.
- In `frontend/src/components/ChronicleView.tsx`:
  - When `turnInFlight && pendingAction` is present:
    - Render a pending turn beat at the bottom of the chronicle containing:
      1. Player action badge (e.g. `[DO]`, `[SAY]`, `[STORY]`, `[ROLL]`) with `pendingAction.text`.
      2. If `streamedProse` is empty: an animated drafting card displaying a pulsing amber/stone indicator with an animated spinner/feather icon and text: *"The narrator is drafting the scene..."*
      3. If `streamedProse` has content: render the streaming prose directly inside the beat using `TurnSegments`.
    - Smoothly auto-scroll to the bottom of the chronicle container when `pendingAction` mounts or when chunks arrive.
  - When the final `turn` event completes:
    - The turn is added to `chronicle`, `pendingAction` is cleared to `null`, and `streamedProse` resets, seamlessly swapping the pending card with the authoritative turn record.

---

### 2.2 Continuity Note Creation Without Advancing Turns

#### Current State
- When continuity verification identifies an unknown speaker or named entity (`RuleUnknownEntity`, `RuleUnresolvedSpeaker`), clicking "Correct" invokes `handleCorrect(note)` which calls `handleActionSubmit('GM', '/gm ' + note)`.
- This triggers a full orchestrator generation, causing:
  - An unwanted turn to be appended to `history.jsonl`.
  - Generation of new story prose by the GM model.
  - Interruption of ongoing audio playback.

#### Target Behavior
- Helper parser: `parseMissingEntityName(note: string): string | null`
  - Detects patterns like `"<name>" speaks but has no note`, `"<name>" is named but has no note`, `"<name>" speaks but is not a known character`.
  - Returns the extracted name (e.g. `"Lady Evelyn"` or `"Evelyn"`).
- In `frontend/src/App.tsx` and `frontend/src/components/ChronicleView.tsx`:
  - When clicking "Correct" on a finding where `parseMissingEntityName(note)` finds an entity name:
    - Open a lightweight modal dialog: **"Add Note for '<EntityName>'?"**
    - The modal presents three options:
      1. **Quick Create**:
         - Derives slug ID via `slugify(name)`.
         - Writes an initial entity note with frontmatter (`id`, `name`, `type: character`, and default voice profile) via `client.saveEntity(id, markdown)`.
         - Calls `client.addressFinding(turnNumber, 'continuity')` to mark the finding addressed.
         - Calls `refreshCorpus()` to refresh the entity index and graph.
         - Does NOT trigger a turn submission and does NOT stop audio playback.
      2. **Edit in Codex**:
         - Creates the starter note in memory (or creates it on disk) and opens the Codex drawer directly focused on that entity note.
         - Marks the finding addressed.
         - Leaves audio playback running undisturbed.
      3. **Cancel**: Dismisses modal with no changes.
  - If the finding is not a missing entity note (e.g. `location-drift`, `state-contradiction`), clicking "Correct" pre-fills the ActionConsole with the `/gm` directive or prompts with a director correction rather than immediately submitting an automatic turn.

---

### 2.3 Codex Drawer Sizing, Maximization & Collapsible Entity List

#### Current State
- `Drawers.tsx` defines size presets: `md: max-w-md`, `lg: max-w-lg`, `xl: max-w-xl` (576px).
- `CodexDrawer.tsx` has a side-by-side flex layout with a fixed `md:w-64` (256px) sidebar and note editor inside the 576px drawer.
- The editor textarea and controls are squashed into ~250px, forcing horizontal scrolling on dropdowns and tags.

#### Target Behavior
1. **Drawer Sizing & Maximize Toggle (`frontend/src/components/Drawers.tsx`)**:
   - Update `sizeClasses`:
     - `md`: `max-w-md` (28rem / 448px)
     - `lg`: `max-w-lg` (32rem / 512px)
     - `xl`: `max-w-3xl` (48rem / 768px)
     - `full`: `max-w-full w-full`
   - Add local state `isMaximized: boolean` inside `Drawers`.
   - In the drawer header, add a maximize/restore button (using `Maximize2` and `Minimize2` icons from `lucide-react`) next to the close button:
     - Toggling switches the drawer container between its assigned size class (`xl`) and `w-full max-w-full`.
2. **Collapsible Entity Sidebar (`frontend/src/components/CodexDrawer.tsx`)**:
   - Add state: `isSidebarOpen: boolean` (defaults to `false` when an entity is already selected, or `true` if no entity is selected yet).
   - In the note editor top toolbar, add a toggle button:
     - `Browse Notes ({count})` with `PanelLeft` / `BookOpen` icon.
     - Clicking toggles `isSidebarOpen`.
   - When `isSidebarOpen` is `false`:
     - The `<aside>` entity list is hidden (`hidden`).
     - The editor (`<div className="flex-1 min-w-0 min-h-0">`) takes 100% of the drawer width.
   - When `isSidebarOpen` is `true`:
     - The `<aside>` entity list is displayed alongside or as an overlay on mobile, smoothly allowing search, filter by tag/type, and note selection.
     - Selecting an entity closes or collapses the sidebar on narrow screens or keeps it open on maximized screens.
   - Zero horizontal scrolling: all selects, voice archetypes, and textareas fit naturally with `overflow-x-hidden`.

---

## 3. Data Flow & Testing Strategy

### 3.1 Data Flow
- **Pending Turn**: Action Submit -> `setPendingAction(action)` + `setTurnInFlight(true)` -> `ChronicleView` renders pending beat -> SSE chunks update `streamedProse` -> SSE turn completes -> `setChronicle` + `setPendingAction(null)` + `refreshCorpus()`.
- **Note Correction**: Continuity finding -> Click "Correct" -> Regex extracts entity -> Confirmation modal -> `client.saveEntity` + `client.addressFinding` + `refreshCorpus()`. No turn stream triggered.
- **Codex Drawer**: User opens Codex drawer -> Drawer renders at `max-w-3xl` (or `max-w-full` if maximized) -> Entity sidebar collapsed -> User edits note comfortably without horizontal scroll.

### 3.2 Testing & Verification
- Frontend TypeScript check (`npx tsc --noEmit`) and Vite build (`npm run build`).
- Go unit test suite (`go test -count=1 ./...`) and lint (`go vet ./...`).
- Manual verification checklist:
  - Turn submit shows pending action and animated drafting card immediately.
  - Streaming prose flows smoothly into the pending beat.
  - Continuity finding "Correct" on unknown entity opens quick modal, creates note without turn generation, and leaves audio playing.
  - Codex drawer opens with `max-w-3xl`, entity list collapsed by default, maximize button toggles full screen width without horizontal scrollbars.
