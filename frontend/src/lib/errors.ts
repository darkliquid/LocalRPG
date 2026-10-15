// errorMessage renders a caught value for the player. A thrown value is unknown
// by definition, so this is the one place the app decides what to say about one:
// an Error's message when there is one, and a sentence when there is not, rather
// than the string "undefined" or "[object Object]".
export function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : 'Unexpected error';
}
