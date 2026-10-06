// isCharacterType reports whether a type names a speaking being. It mirrors
// entity.IsCharacterType in pkg/entity/entity.go, so the codex and the chronicle
// agree on which entities have a portrait.
export function isCharacterType(type?: string): boolean {
  switch ((type ?? '').trim().toLowerCase()) {
    case 'character':
    case 'npc':
    case 'person':
    case 'creature':
      return true;
    default:
      return false;
  }
}
