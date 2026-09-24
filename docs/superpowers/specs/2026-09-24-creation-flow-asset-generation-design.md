# AI Asset Generation in Creation Flows Design Specification

**Date:** 2026-09-24
**Status:** Approved
**Scope:** New Campaign Flow, Worlds Studio
**Related:** AI Asset Generation in Creation Flows

## 1. Overview & Goals

Currently, the LocalRPG application allows AI generation of assets (banners and icons) for existing entities (Games and Worlds). However, users lack the ability to generate assets during the creation flow of new campaigns and worlds. Furthermore, the default state of the Worlds Studio is confusingly populated with content from the "Ashen Reach" reference template rather than a blank slate.

This specification outlines the technical design to:
1. Enable AI asset generation during the New Campaign creation flow via a new stateless preview endpoint.
2. Update the Worlds Studio to start with a blank slate, make the reference template opt-in, and integrate AI asset generation for unsaved worlds.

### Success Criteria
- Users can click an "AI Gen" button next to banner and icon fields in `NewCampaignModal` and `WorldsStudio` (for unsaved worlds).
- The system returns generated images as preview blobs without saving them directly to the filesystem (to prevent orphaned files if the user abandons the form).
- Submitting the forms successfully uploads the blobs using the existing upload mechanisms.
- `WorldsStudio` loads a blank template by default, with an explicit "Load Reference Template" button to pre-fill the "Ashen Reach" example.
- The same prompt generation logic is shared between the existing generation methods and the new preview endpoint.

### Non-Goals
- Generating assets for entities other than games (campaigns) and worlds.
- Modifying the underlying AI generation logic or providers.
- Changing the existing flow for already-saved campaigns or worlds.

## 2. Architecture

The core of this design involves a new backend endpoint to generate assets in-memory and return them directly to the client as raw bytes, bypassing the standard saving mechanism.

1.  **Backend Preview Endpoint:** A new `POST /api/generate-asset-preview` endpoint will receive context (kind, name, description, art_style) and return raw image bytes (`[]byte`) with an appropriate `Content-Type`.
2.  **Prompt Logic Extraction:** The prompt formulation currently embedded in `GenerateGameAsset` and `GenerateWorldAsset` will be extracted into a shared helper function to ensure consistency across all generation paths.
3.  **Frontend Blob Handling:** The frontend `APIClient` will consume the preview endpoint and return a `Blob`. Forms will hold this `Blob` in their state (identical to manual file uploads) and render it for preview. On form submission, the `Blob` is uploaded using existing endpoints (`uploadGameAsset`, `uploadWorldAsset`).

## 3. Backend Changes

### 3.1. New Endpoint: `/api/generate-asset-preview`
Add a new route to `pkg/gui/server.go`:
```go
// In handleGeneralRoutes or similar setup function
r.POST("/api/generate-asset-preview", s.handleGenerateAssetPreview)
```

### 3.2. Data Transfer Object (DTO)
Define the request struct:
```go
type GenerateAssetPreviewRequest struct {
    Kind        string `json:"kind"` // "banner" or "icon"
    Name        string `json:"name"`
    Description string `json:"description"`
    ArtStyle    string `json:"art_style"`
    Genre       string `json:"genre,omitempty"` // For world context if applicable
}
```

### 3.3. Service Method
Implement `GenerateAssetPreview` in `pkg/gui/service.go`:
```go
func (s *Service) GenerateAssetPreview(ctx context.Context, req GenerateAssetPreviewRequest) ([]byte, string, error) {
    // 1. Build prompt using extracted helper
    prompt := buildAssetPrompt(req.Kind, req.Name, req.Description, req.ArtStyle, req.Genre)
    
    // 2. Call AI provider (e.g., s.aiManager.GenerateImage)
    // 3. Return image bytes and content type
}
```

### 3.4. Prompt Builder Refactoring
Extract prompt logic from existing methods (`GenerateGameAsset`, `GenerateWorldAsset`) in `pkg/gui/service.go` into:
```go
func buildAssetPrompt(kind, name, description, artStyle, genre string) string {
    // Shared prompt template logic based on kind ("banner" or "icon")
    // and provided contextual parameters.
}
```
Refactor `GenerateGameAsset` and `GenerateWorldAsset` to utilize this new helper.

## 4. Frontend Changes

### 4.1. API Client Updates
In `frontend/src/api/client.ts`, add the preview generation method:
```typescript
async generateAssetPreview(kind: 'banner' | 'icon', name: string, description: string, artStyle: string): Promise<Blob> {
    const res = await fetch('/api/generate-asset-preview', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind, name, description, art_style: artStyle })
    });
    // Check res.ok, handle errors
    return await res.blob();
}
```
Update `frontend/src/types.ts` to reflect the new API client method if an interface is used.

### 4.2. NewCampaignModal.tsx
- **UI:** Add ✨ AI Gen buttons (using the `Sparkles` icon from `lucide-react`) next to the upload buttons for Banner and Icon.
- **State Integration:** The `onClick` handler should call `APIClient.generateAssetPreview`. The parameters (`name`, `description`, `artStyle`) will be sourced from the selected world's data and the entered campaign name.
- **Preview:** Set the returned `Blob` to the existing state variables (`bannerFile` / `iconFile`). The existing code that generates object URLs for these files will handle the preview automatically.
- **Validation:** Disable the AI Gen buttons if no world is selected or if the campaign name is empty.

### 4.3. WorldsStudio.tsx
- **Unsaved Worlds:** For worlds that have not been saved, the AI Gen buttons for Banner and Icon will call `APIClient.generateAssetPreview`.
- **Saved Worlds:** Continue using the existing `APIClient.generateWorldAsset` functionality.
- **State Handling:** Similar to `NewCampaignModal`, store the preview `Blob` into local state and render it. Form submission leverages existing `uploadWorldAsset`.

## 5. World Default State

The current default state in `WorldsStudio` relies on `REFERENCE_WORLD_TEMPLATE` (the "Ashen Reach" template), causing confusion for users who expect a blank slate.

- **Blank Default:** Update `WorldsStudio` to initialize with empty strings for `name`, `description`, `genre`, `art_style`, `lore_prompt`, and `slug`, along with empty arrays for entities.
- **`STARTER_ENTITY_TEMPLATE`:** Update generic entities inside `frontend/src/templates/referenceTemplates.ts` to use placeholder/generic names instead of "Ashen Reach"-specific content.
- **Opt-in Reference Template:** 
  - Keep `REFERENCE_WORLD_TEMPLATE` in `referenceTemplates.ts`.
  - Add a "Load Reference Template" button (using `BookOpen` icon) in the Worlds Studio UI.
  - When clicked, this button overwrites the current form state with `REFERENCE_WORLD_TEMPLATE`.
  - Modify `handleNewWorld` to clear state fields entirely.

## 6. Error Handling

- **Missing Provider/Config:** If the AI provider is not configured, the backend should return a standard error response (e.g., HTTP 400 or 503). The frontend should catch this and display a user-friendly error message (e.g., via a toast).
- **Generation Failure:** Catch timeout and failure errors from the provider, returning a generic "Failed to generate asset" to the frontend.
- **Insufficient Context:** The backend must validate that required fields (`kind`, `name`) are present. The frontend will pre-emptively disable UI elements when context is lacking.

## 7. Testing & Verification

1.  **Backend Unit Tests:** Verify `buildAssetPrompt` produces expected strings for different input combinations.
2.  **Preview Endpoint Validation:** Ensure `POST /api/generate-asset-preview` correctly returns a blob and proper Content-Type without modifying the file system.
3.  **UI Verification:**
    - Open `NewCampaignModal`, fill required fields, and click AI Gen. Confirm the preview shows and submission succeeds.
    - Open `WorldsStudio`, verify it starts completely empty.
    - Click "Load Reference Template", confirm the form populates with Ashen Reach.
    - Click "New World", confirm the form clears.
    - Generate assets for an unsaved world in Worlds Studio, confirm preview displays and submission succeeds.
