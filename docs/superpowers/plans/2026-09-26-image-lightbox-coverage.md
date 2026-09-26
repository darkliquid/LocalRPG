# Image Lightbox Coverage Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every campaign/world/chronicle image open the shared full-size `ImageLightbox` on click, through one small reusable hook.

**Architecture:** Add a `useLightbox` hook that owns `{ src, alt } | null` state, then wire it into six existing components. Each component renders a single `<ImageLightbox onClose={closeLightbox} />` and turns the relevant image into a real zoom control. Small images inside clickable cards get a sibling magnify button so no button is nested inside another button.

**Tech Stack:** React 19, TypeScript (strict), Tailwind v4, `lucide-react`.

**Spec:** `docs/superpowers/specs/2026-09-26-image-lightbox-coverage-design.md`

## Global Constraints

- TypeScript `strict`, `noUnusedLocals`, `noUnusedParameters`: remove unused imports/params or the build fails.
- Existing `ImageLightbox` props are exactly `{ src: string; alt: string; onClose: () => void }` (`frontend/src/components/ImageLightbox.tsx`).
- No new dependencies; icons come from `lucide-react`.
- Do not nest a `<button>` inside a `<button>`; use sibling controls in card layouts.
- No lightbox for the standalone web-export viewer.
- Typecheck gate for every task: `mise run test:frontend` (which runs `npx tsc --noEmit` in `frontend/`). If mise fails, run `npx tsc --noEmit` with working directory `frontend/`.
- Do not commit unless the user asks.

---

## File Map

- Create: `frontend/src/hooks/useLightbox.ts` — shared lightbox state.
- Modify: `frontend/src/components/launcher/CampaignHeroStage.tsx` — bottom-left campaign icon.
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx` — banner + icon.
- Modify: `frontend/src/components/launcher/WorldGallery.tsx` — card banner/icon magnify.
- Modify: `frontend/src/components/launcher/WorldFlyout.tsx` — dock icon magnify.
- Modify: `frontend/src/components/WorldsStudio.tsx` — banner/icon previews.
- Modify: `frontend/src/components/ChronicleView.tsx` — scene illustration + location art.

---

### Task 1: Shared `useLightbox` hook

**Files:**
- Create: `frontend/src/hooks/useLightbox.ts`

**Interfaces:**
- Produces: `useLightbox(): { lightbox: LightboxTarget | null; openLightbox(src: string, alt: string): void; closeLightbox(): void }`; exports `type LightboxTarget = { src: string; alt: string }`.

- [x] **Step 1: Create the hook**

```ts
import { useState } from 'react';

export interface LightboxTarget {
  src: string;
  alt: string;
}

export interface UseLightboxResult {
  lightbox: LightboxTarget | null;
  openLightbox: (src: string, alt: string) => void;
  closeLightbox: () => void;
}

export function useLightbox(): UseLightboxResult {
  const [lightbox, setLightbox] = useState<LightboxTarget | null>(null);
  return {
    lightbox,
    openLightbox: (src, alt) => setLightbox({ src, alt }),
    closeLightbox: () => setLightbox(null),
  };
}
```

- [x] **Step 2: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0, no errors.

---

### Task 2: Campaign hero title-card icon

**Files:**
- Modify: `frontend/src/components/launcher/CampaignHeroStage.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

Add after the existing `lucide-react` import:

```tsx
import { useLightbox } from '../../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';
```

- [x] **Step 2: Add hook and captured URL**

In the component body, immediately before `return (`:

```tsx
  const iconURL = game?.icon_url;
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
```

- [x] **Step 3: Make the icon a zoom control**

Replace the icon container (currently `CampaignHeroStage.tsx:170-176`) with:

```tsx
            <div className="w-[52px] h-[52px] rounded-xl overflow-hidden flex-shrink-0 shadow-lg border border-white/10">
              {iconURL ? (
                <button
                  type="button"
                  onClick={() => openLightbox(iconURL, game.name)}
                  className="w-full h-full cursor-zoom-in"
                  title={`View full size: ${game.name}`}
                  aria-label={`View full size: ${game.name}`}
                >
                  <img src={iconURL} alt={game.name} className="w-full h-full object-cover" />
                </button>
              ) : (
                <ProceduralIcon id={game.id} name={game.name} size={52} className="w-full h-full rounded-none" />
              )}
            </div>
```

- [x] **Step 4: Render the lightbox**

Before the closing `</main>`:

```tsx
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
```

- [x] **Step 5: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 3: Campaign settings banner + icon

**Files:**
- Modify: `frontend/src/components/launcher/CampaignSettingsModal.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

```tsx
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';
```

- [x] **Step 2: Add hook and captured URLs**

Add alongside the other state:

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
```

After the early return that guards `!isOpen || !game` (find `if (!isOpen || !game) return null;`), add:

```tsx
  const bannerURL = game.banner_url;
  const iconURL = game.icon_url;
```

- [x] **Step 3: Make the banner zoomable**

Replace the banner preview (`:294-300`) with:

```tsx
                <div className="h-24 rounded-xl overflow-hidden relative border border-white/10">
                  {bannerURL ? (
                    <button
                      type="button"
                      onClick={() => openLightbox(bannerURL, 'Campaign banner')}
                      className="w-full h-full cursor-zoom-in"
                      title="View full size banner"
                      aria-label="View full size banner"
                    >
                      <img src={bannerURL} alt="Banner" className="w-full h-full object-cover" />
                    </button>
                  ) : (
                    <ProceduralBanner id={game.id} name={game.name} className="w-full h-full" />
                  )}
                </div>
```

- [x] **Step 4: Make the icon zoomable**

Replace the icon preview (`:333-339`) with:

```tsx
                <div className="h-24 rounded-xl overflow-hidden relative border border-white/10 flex items-center justify-center bg-stone-900">
                  {iconURL ? (
                    <button
                      type="button"
                      onClick={() => openLightbox(iconURL, 'Campaign icon')}
                      className="w-16 h-16 rounded-xl overflow-hidden cursor-zoom-in"
                      title="View full size icon"
                      aria-label="View full size icon"
                    >
                      <img src={iconURL} alt="Icon" className="w-full h-full object-cover" />
                    </button>
                  ) : (
                    <ProceduralIcon id={game.id} name={game.name} size={64} />
                  )}
                </div>
```

- [x] **Step 5: Render the lightbox**

Before the component's final closing `</div>` (after the Footer block):

```tsx
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
```

- [x] **Step 6: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 4: World gallery cards

**Files:**
- Modify: `frontend/src/components/launcher/WorldGallery.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

```tsx
import { useLightbox } from '../../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';
```

Add `Maximize2` to the existing `lucide-react` import.

- [x] **Step 2: Add hook inside `WorldCard` and restructure the root**

The card root is currently a `<button>` (`:22-82`); a magnify button cannot nest inside it. Change the root to a `group relative` wrapper, keep the selectable button as a child with `w-full`, and add a sibling magnify button.

At the top of `WorldCard`, before `return (`:

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
  return (
    <div className="group relative">
      <button
        onClick={onSelect}
        className="w-full flex flex-col text-left rounded-3xl overflow-hidden border border-white/10 hover:border-purple-400/70 bg-stone-900/60 hover:bg-stone-900 shadow-xl hover:shadow-2xl hover:-translate-y-0.5 transition-all duration-200 cursor-pointer"
        aria-label={`Create a campaign in ${world.name}`}
      >
        {/* ...existing card body unchanged... */}
      </button>
      {world.banner_url || world.icon_url ? (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            openLightbox((world.banner_url || world.icon_url)!, world.name);
          }}
          className="absolute top-3 right-3 w-8 h-8 rounded-lg bg-black/60 hover:bg-black/80 border border-white/15 flex items-center justify-center text-white opacity-0 group-hover:opacity-100 focus:opacity-100 transition-opacity cursor-zoom-in"
          title={`View full size: ${world.name}`}
          aria-label={`View full size: ${world.name}`}
        >
          <Maximize2 className="w-4 h-4" />
        </button>
      ) : null}
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
    </div>
  );
```

The existing card body (banner, icon/name overlay, body text, tags) stays byte-for-byte the same inside the new `<button>`; only the outer element and the closing tags change, plus the two new siblings.

- [x] **Step 3: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 5: World flyout dock icons

**Files:**
- Modify: `frontend/src/components/launcher/WorldFlyout.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

```tsx
import { useLightbox } from '../../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';
```

Add `Maximize2` to the existing `lucide-react` import.

- [x] **Step 2: Add hook**

In `WorldFlyout`, after `if (!isOpen) return null;`:

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
```

Hooks must run unconditionally: move the hook above the `if (!isOpen) return null;` guard instead if the lint/build complains. Preferred placement is directly after the destructured props, before the guard:

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
  if (!isOpen) return null;
```

- [x] **Step 3: Add a magnify button per world and render the lightbox**

Inside the per-world `div` (`:38`), after the existing select button:

```tsx
            {world.icon_url && (
              <button
                type="button"
                onClick={() => openLightbox(world.icon_url!, `${world.name} icon`)}
                className="absolute -top-1 -right-1 w-4 h-4 rounded-full bg-black/80 border border-white/20 flex items-center justify-center text-white opacity-0 group-hover:opacity-100 transition-opacity cursor-zoom-in"
                title={`View full size: ${world.name}`}
                aria-label={`View full size: ${world.name}`}
              >
                <Maximize2 className="w-2.5 h-2.5" />
              </button>
            )}
```

Before the flyout's final closing `</div>`:

```tsx
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
```

- [x] **Step 4: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 6: World studio banner/icon previews

**Files:**
- Modify: `frontend/src/components/WorldsStudio.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

Add after the `AIGenerateButton` import:

```tsx
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from '../ImageLightbox';
```

- [x] **Step 2: Add hook**

Add alongside the other state (near `bannerPreview`/`iconPreview`):

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
```

- [x] **Step 3: Banner preview click opens the lightbox**

Replace the banner preview wrapper `onClick` (`:859-861`) and image (`:863-869`) with:

```tsx
                  <div
                    onClick={() =>
                      bannerPreview ? openLightbox(bannerPreview, 'World banner') : bannerInputRef.current?.click()
                    }
                    className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                      bannerPreview ? 'cursor-zoom-in' : 'cursor-pointer'
                    }`}
                  >
                    {bannerPreview ? (
                      <img
                        src={bannerPreview}
                        alt="Banner Preview"
                        className="w-full h-full object-cover"
                        onError={() => setBannerPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload banner</span>
                    )}
                  </div>
```

- [x] **Step 4: Icon preview click opens the lightbox**

Replace the icon preview wrapper `onClick` (`:912-914`) and image (`:916-922`) with:

```tsx
                  <div
                    onClick={() =>
                      iconPreview ? openLightbox(iconPreview, 'World icon') : iconInputRef.current?.click()
                    }
                    className={`h-24 rounded-lg overflow-hidden border border-stone-800 bg-stone-900/40 flex items-center justify-center ${
                      iconPreview ? 'cursor-zoom-in' : 'cursor-pointer'
                    }`}
                  >
                    {iconPreview ? (
                      <img
                        src={iconPreview}
                        alt="Icon Preview"
                        className="w-16 h-16 rounded-xl object-cover"
                        onError={() => setIconPreview(null)}
                      />
                    ) : (
                      <span className="text-stone-600 text-xs">Click to upload icon</span>
                    )}
                  </div>
```

- [x] **Step 5: Render the lightbox**

Before the component's final closing `</div>`:

```tsx
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
```

- [x] **Step 6: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 7: Chronicle scene illustration and location art

**Files:**
- Modify: `frontend/src/components/ChronicleView.tsx`

**Interfaces:**
- Consumes: `useLightbox` (Task 1), `ImageLightbox`.

- [x] **Step 1: Add imports**

```tsx
import { useLightbox } from '../hooks/useLightbox';
import { ImageLightbox } from '../components/ImageLightbox';
```

Use `./ImageLightbox` (same directory), not `../components/`:

```tsx
import { ImageLightbox } from './ImageLightbox';
```

- [x] **Step 2: Add hook**

In `ChronicleView`, after `const bottomRef = useRef(...)`:

```tsx
  const { lightbox, openLightbox, closeLightbox } = useLightbox();
```

- [x] **Step 3: Capture per-turn URLs**

Inside the `beats.map(({ turn, isSceneChange }, index) => {` callback, before `return (`:

```tsx
          const imageURL = turn.image_url;
          const locationArtURL = turn.location_art_url;
```

- [x] **Step 4: Make the scene illustration zoomable**

Replace `:86-91` with:

```tsx
              {/* Scene Illustration if available */}
              {imageURL && (
                <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                  <button
                    type="button"
                    onClick={() => openLightbox(imageURL, 'Scene illustration')}
                    className="block w-full cursor-zoom-in"
                    title="View full size scene illustration"
                    aria-label="View full size scene illustration"
                  >
                    <img src={imageURL} alt="Scene illustration" className="w-full object-cover max-h-96" />
                  </button>
                </div>
              )}
```

- [x] **Step 5: Make the location art zoomable**

Replace `:93-107` with:

```tsx
              {/* Scene art, when the party has moved somewhere new */}
              {locationArtURL && isSceneChange && (
                <div className="my-4 rounded-xl overflow-hidden border border-white/10 shadow-2xl">
                  <button
                    type="button"
                    onClick={() => openLightbox(locationArtURL, turn.location_name || 'Scene')}
                    className="block w-full cursor-zoom-in"
                    title="View full size scene art"
                    aria-label="View full size scene art"
                  >
                    <img
                      src={locationArtURL}
                      alt={turn.location_name || 'Scene'}
                      className="w-full object-cover max-h-96"
                    />
                  </button>
                  {turn.location_name && (
                    <div className="px-3 py-2 text-xs font-sans tracking-widest text-stone-400 uppercase">
                      {turn.location_name}
                    </div>
                  )}
                </div>
              )}
```

- [x] **Step 6: Render the lightbox**

Before the component's final closing `</div>` (the root returned by `ChronicleView`):

```tsx
      {lightbox && (
        <ImageLightbox src={lightbox.src} alt={lightbox.alt} onClose={closeLightbox} />
      )}
```

- [x] **Step 7: Typecheck**

Run: `mise run test:frontend`
Expected: exit 0.

---

### Task 8: Full verification

**Files:** none.

- [x] **Step 1: Typecheck the whole app**

Run: `mise run test:frontend`
Expected: exit 0.

- [x] **Step 2: Build the frontend bundle**

Run: `mise run build:frontend`
Expected: `tsc` + `vite build` succeed; `.gitkeep` preserved.

- [x] **Step 3: Manual verification checklist**

Launch `mise run dev:gui` (or `mise run dev:frontend` against a running backend) and confirm:
- Launcher bottom-left campaign icon -> lightbox; Escape and backdrop close it.
- Campaign settings: banner and icon -> lightbox.
- World studio: banner/icon previews -> lightbox, Upload buttons still open the file picker, empty zones still upload.
- World gallery card: magnify button -> lightbox; clicking the card body still selects the world.
- World flyout: magnify button -> lightbox; clicking the icon still selects the world.
- Chronicle: scene illustration and location art -> lightbox.

---

## Self-Review

**Spec coverage:** The spec's five surfaces are Tasks 2-7; the shared hook is Task 1; the "no nested button" and accessibility requirements are encoded inline. The launcher `CampaignGallery` is intentionally not in the spec's table and is out of scope; the world gallery/flyout are covered.

**Placeholder scan:** No TBD/TODO; every code step shows real code.

**Type consistency:** `useLightbox` returns `{ lightbox, openLightbox, closeLightbox }` and every task uses those exact names. `LightboxTarget` is `{ src: string; alt: string }`, matching `ImageLightbox`'s props. Captured `iconURL`/`bannerURL`/`imageURL`/`locationArtURL` constants preserve TypeScript narrowing inside click callbacks.
