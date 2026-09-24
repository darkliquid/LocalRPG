# On-Demand Text Generation Design

**Date:** 2026-09-24
**Status:** Approved
**Scope:** Backend and Frontend AI text generation integration
**Related:** GUI Text Fields, AI Provider Integration

## 1. Overview & Goals

All text fields across all forms in the GUI should support on-demand AI text generation. This includes individual field generation and a 'generate all' mode per form. Currently, the backend has an endpoint for character generation (`POST /api/character/generate`), but the frontend integration was dropped during a UI refactor. Other forms (world, system, campaign settings) currently lack text generation support entirely.

**Goals:**
- Provide a unified backend endpoint for on-demand text generation across different form types.
- Create a reusable frontend component (`AIGenerateButton`) for inline field generation.
- Implement a form-level 'Generate All' feature to populate empty fields.
- Ensure contextual coherence by passing existing form data as context.
- Maintain backward compatibility for the existing `POST /api/character/generate` endpoint.

**Non-Goals:**
- Removing or deprecating the old `POST /api/character/generate` endpoint (must remain for backward compatibility).
- Automatically generating non-text assets (like images or audio) in this iteration.

**Success Criteria:**
- The new `POST /api/generate-text` endpoint correctly serves individual and 'all' field requests for character, world, system, and campaign forms.
- Reusable `AIGenerateButton` is implemented and integrated into all specified forms.
- 'Generate All' buttons are available in all specified forms and populate only empty/unfilled fields.
- Context injection works, producing coherent text based on other field values.

## 2. Architecture

The feature introduces a generic text generation API endpoint and reusable React components that integrate with existing forms.

- **Backend Endpoint:** `POST /api/generate-text` handles generation for all form types. It routes to specific prompt templates and LLM roles based on the requested form type.
- **Frontend Component:** `AIGenerateButton` acts as the primary inline trigger for individual fields. A separate 'Generate All' button handles form-wide population.
- **API Client:** Updates to `client.ts` to support the new endpoint.
- **Integration Points:** `NewCampaignModal.tsx`, `CampaignSettingsModal.tsx`, `WorldsStudio.tsx`, and `SystemsStudio.tsx`.

## 3. Backend Changes

### New Endpoint
- **Path:** `POST /api/generate-text`
- **File:** `pkg/gui/text_generate.go`
- **Handler & DTOs:**
  ```go
  type GenerateTextRequest struct {
      FormType string            `json:"form_type"` // "character", "world", "system", "campaign"
      FieldName string            `json:"field_name"` // string or "_all"
      Context  map[string]string `json:"context"`
      WorldID  string            `json:"world_id,omitempty"`
      SystemID string            `json:"system_id,omitempty"`
      Seed     string            `json:"seed,omitempty"`
  }

  type GenerateTextResponse struct {
      Fields      map[string]string `json:"fields"`
      GeneratedBy string            `json:"generated_by"`
  }
  ```

### Service Implementation
- **Method:** `Service.GenerateText(ctx context.Context, req GenerateTextRequest) (*GenerateTextResponse, error)`
- **Prompt Routing:** Routes to different prompt templates based on `req.FormType`.
- **Role Routing:** Uses `"character"` role for the character form type, and `"gm"` fallback for all others. Overridable via config.
- **Reusability:** Reuses the existing `decodeGeneratedValues` function from `character_generate.go` to parse LLM outputs reliably.
- **Backward Compatibility:** Keep `POST /api/character/generate` and its underlying functions intact in `pkg/gui/character_generate.go`.

## 4. Frontend Changes

### Reusable Components
- **File:** `frontend/src/components/ui/AIGenerateButton.tsx`
- **Props:**
  ```typescript
  interface AIGenerateButtonProps {
      formType: string;
      fieldName: string;
      getContext: () => Record<string, string>;
      onGenerated: (value: string) => void;
      disabled?: boolean;
      className?: string;
  }
  ```
- **UI:** A small, compact inline button using the `Sparkles` icon from `lucide-react`. Shows a loading spinner during API calls.
- **Error Handling:** Shows a brief error toast/state on failure without throwing uncaught exceptions.

### Form-Level 'Generate All' Button
- Uses the `Wand2` icon from `lucide-react`.
- Triggers `APIClient.generateText(formType, '_all', context)`.
- Updates all empty/unfilled fields with generated values, ensuring user-entered values are not overwritten.
- Shows a loading state during the generation process.

### API Client
- **File:** `frontend/src/api/client.ts`
- **Add Method:** `APIClient.generateText(formType: string, fieldName: string, context: Record<string, string>, opts?: any): Promise<GenerateTextResponse>`
- **Types:** Update `frontend/src/types.ts` with `GenerateTextRequest` and `GenerateTextResponse`.

### Form Integrations
- **`NewCampaignModal.tsx`:** Add `AIGenerateButton` to character fields (name, age, gender, pronouns, appearance, background, opening_prompt, start_location) and the form-level 'Generate All' button.
- **`CampaignSettingsModal.tsx`:** Add `AIGenerateButton` to name and description fields.
- **`WorldsStudio.tsx`:** Add `AIGenerateButton` to name, description, genre, art_style, lore_prompt fields and 'Generate All' button.
- **`SystemsStudio.tsx`:** Add `AIGenerateButton` to name, description, rules_prompt fields and 'Generate All' button.

## 5. Prompt Template Details

Each form type uses specific prompt templates structured to ensure quality generation:

1. **System Prompt:** Explains the persona (e.g., character inventor, world builder, system designer, campaign designer).
2. **Field Descriptions:** Specifies what each field should contain.
   - **character:** Dynamically looks up `CharacterCreationFields` from the system manifest using `engine.CharacterFields(sysManifest)` for schema definitions. Includes world lore for flavor. Core fields: name, age, gender, pronouns, appearance, background, plus custom system fields.
   - **world:** Fields: name, description, genre, art_style, lore_prompt. Context uses existing field values.
   - **system:** Fields: name, description, rules_prompt. Generation is technically focused.
   - **campaign:** Fields: name, description, start_location, opening_prompt. Context uses world name/description and system name.
3. **Context Injection:** Injects current form field values so generation aligns with already established elements.
4. **Target Handling (`_all` vs Single):**
   - For `_all`, instructs the LLM to generate all fields as a JSON object.
   - For a single field, instructs the LLM to generate just that specific field, returning it in the specified JSON structure.
5. **Seed Handling:** If `seed` is provided, instructs the LLM to extend, improve, or rework the existing text rather than generating from scratch.

## 6. Error Handling

- **Provider Not Configured:** Returns a graceful `400 Bad Request` or `503 Service Unavailable` with a clear message indicating that an AI provider is required. The frontend displays this as a non-intrusive toast notification.
- **Generation Failure:** API errors (timeouts, rate limits, invalid formats) are caught. The frontend resets the loading state and displays an error toast.
- **Empty Results / Parsing Errors:** The `decodeGeneratedValues` function ensures robust parsing. If it fails or returns empty fields, the backend returns an error. The frontend will not overwrite existing values with empty strings on failure.

## 7. Testing & Verification

- **Backend Unit Tests:** Test `Service.GenerateText` with mocked LLM responses for each `FormType`. Test the parsing of single fields and `_all`. Test error propagation.
- **Frontend Component Tests:** Verify `AIGenerateButton` triggers the correct API call, displays loading states, handles successful generation, and handles errors without crashing.
- **Integration Tests:** Verify forms correctly aggregate context and update fields upon successful generation without overwriting user data during 'Generate All'.
- Verify that `POST /api/character/generate` still functions correctly with legacy clients.
