// The studio's own types, shared by the container and its views. They describe
// the unsaved draft and what the master column has selected, neither of which is
// a wire type: the server knows nothing about a draft's local id.

export type SystemSelection = { kind: 'saved'; id: string } | { kind: 'draft' } | null;

export interface SystemDraft {
  localId: string;
  dirty: boolean;
}
