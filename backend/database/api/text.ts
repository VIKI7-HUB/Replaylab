/** `%term%` for ILIKE, with the user's own % and _ matched literally. */
export function likePattern(term: string): string {
  return `%${term.replace(/[\\%_]/g, (c) => `\\${c}`)}%`;
}

/**
 * A to_tsquery('simple', ...) string matching every word of `term` (dotted and
 * hyphenated words such as 3.11 or gpt-4o stay whole, as Postgres parses them);
 * words of 3+ characters also match as prefixes (typeahead). Only letters, digits,
 * dots and hyphens survive, so the result is always valid tsquery syntax. Null when `term` has
 * no words.
 */
export function tsQuery(term: string): string | null {
  const words = term.toLowerCase().match(/[\p{L}\p{N}]+(?:[.-][\p{L}\p{N}]+)*/gu)?.slice(0, 8) ?? [];
  if (words.length === 0) return null;
  return words.map((w) => (w.length >= 3 ? `${w}:*` : w)).join(' & ');
}
