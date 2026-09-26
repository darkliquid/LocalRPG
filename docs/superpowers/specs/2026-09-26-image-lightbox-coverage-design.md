# Design Spec: Full-Size Image Lightbox Coverage

**Date:** 2026-09-26
**Status:** Proposed
**Target:** `frontend` (`ImageLightbox.tsx`, `hooks/useLightbox.ts`, `launcher/CampaignHeroStage.tsx`, `launcher/CampaignSettingsModal.tsx`, `WorldsStudio.tsx`, `launcher/WorldGallery.tsx`, `launcher/WorldFlyout.tsx`, `ChronicleView.tsx`)

---

## 1. Executive Summary

Clicking a generated or uploaded image should open a full-size lightbox. Today only the Codex character portrait (`CodexDrawer.tsx:296-310`) and Chronicle speech avatars (`TurnSegments.tsx:70`) wire the shared `ImageLightbox` component; every other image surface renders a plain `<img>` with no zoom affordance:

| Surface | File:line | Image |
|---|---|---|
| Launcher bottom-left title-card icon | `launcher/CampaignHeroStage.tsx:170-176` | `game.icon_url` |
| Campaign settings banner | `launcher/CampaignSettingsModal.tsx:295-299` | `game.banner_url` |
| Campaign settings icon | `launcher/CampaignSettingsModal.tsx:334-338` | `game.icon_url` |
| World studio banner | `WorldsStudio.tsx:863-869` | `bannerPreview` |
| World studio icon | `WorldsStudio.tsx:916-922` | `iconPreview` |
| World gallery banner/icon | `launcher/WorldGallery.tsx:29-45` | `world.banner_url` / `world.icon_url` |
| World flyout icon | `launcher/WorldFlyout.tsx:45-47` | `world.icon_url` |
| Chronicle scene illustration | `ChronicleView.tsx:87-90` | `turn.image_url` |
| Chronicle location art | `ChronicleView.tsx:94-100` | `turn.location_art_url` |

This spec adds the lightbox to all of them, plus a small shared hook so the state boilerplate is defined once.

---

## 2. Approach

### 2.1 Shared `useLightbox` hook

Add `frontend/src/hooks/useLightbox.ts`:

```typescript
export interface LightboxTarget { src: string; alt: string }

export function useLightbox(): {
  lightbox: LightboxTarget | null;
  openLightbox: (src: string, alt: string) => void;
  closeLightbox: () => void;
} {
  const [lightbox, setLightbox] = useState<LightboxTarget | null>(null);
  return {
    lightbox,
    openLightbox: (src, alt) => setLightbox({ src, alt }),
    closeLightbox: () => setLightbox(null),
  };
}
```

Each call site renders one `<ImageLightbox ... onClose={closeLightbox} />` at the end of its JSX, matching the existing `CodexDrawer`/`TurnSegments` pattern. The hook does not replace `ImageLightbox`; it only removes repeated state. Migrating the two existing call sites to the hook is optional and low-risk; leaving them untouched is acceptable.

### 2.2 Interaction contract

- An image that opens a lightbox is wrapped in, or overlaid by, a real `<button>` (preferred) or given `role="button"`, `tabIndex={0}`, an `onClick`, and `onKeyDown` for Enter/Space.
- Styling: `cursor-zoom-in`, and an optional magnify affordance (`Maximize2` from `lucide-react`) shown on hover.
- `aria-label` / `title` states the image being viewed, e.g. `View full size: <name>`.
- The existing `ImageLightbox` already closes on Escape and backdrop click; no change needed there.
- Clicking an image must **not** trigger an enclosing navigation/select handler. Where a wrapper already has its own click behavior, the lightbox control must be a nested button with `e.stopPropagation()`.

### 2.3 Per-surface changes

**Launcher title card** (`CampaignHeroStage.tsx:170-176`): when `game.icon_url` is set, the icon container becomes a `<button>` that calls `openLightbox(game.icon_url, game.name)`. The procedural-icon fallback stays non-clickable. The parent wrapper is not `pointer-events-none` (that applies to the banner overlay), so no overlay change is required.

**Campaign settings modal** (`CampaignSettingsModal.tsx`): the banner (`:295`) and icon (`:334`) previews become zoom controls. Their sibling Upload / AI-Gen buttons keep their existing behavior; only the image itself opens the lightbox.

**World studio** (`WorldsStudio.tsx`): the banner (`:863`) and icon (`:916`) preview wrappers currently open the file picker on click. Split the affordances:
- When a preview exists, clicking the image opens the lightbox; the separate `Upload` button (`:877`, `:930`) still opens the file picker.
- When no preview exists, clicking the empty drop zone still opens the file picker.

**World gallery / flyout** (`WorldGallery.tsx`, `WorldFlyout.tsx`): these cards are themselves clickable (`onSelect`). Add a small magnify button in the corner of the banner/icon that stops propagation and opens the lightbox `src`. This avoids stealing the card's select action.

**Chronicle** (`ChronicleView.tsx`): wrap the scene illustration (`:87-90`) and the scene-change location art (`:94-100`) in zoom buttons using `turn.image_url` / `turn.location_art_url` as `src` and `turn.location_name || 'Scene'` as `alt`. This component is read-only, so wrapping in a button has no conflicting handler.

---

## 3. Non-Goals

- No pan/zoom, carousel, download, or next/previous navigation inside the lightbox.
- No change to upload or AI-generation flows.
- No lightbox for the standalone web-export viewer or the video exporter.
- No new backend endpoints; all sources are already-served URLs.

---

## 4. Test Strategy

1. **Frontend type + build gate**: `npm --prefix frontend run build` (`tsc` strict, `noUnusedLocals`, `noUnusedParameters`) must pass; the hook and every new import are tree-shake checked.
2. **Manual verification**: open the launcher, click the bottom-left campaign icon -> lightbox; open campaign settings, click banner and icon -> lightbox; in world studio click banner/icon -> lightbox while Upload still opens the picker; in world gallery/flyout click the magnify control without selecting the world; in the chronicle click scene art and location art -> lightbox.
3. **Keyboard**: Tab to each control, Enter/Space opens, Escape closes.
4. **Regression**: clicking a world gallery card still selects the world; clicking a campaign settings image does not open the file picker.
