import { z } from 'zod';

export class BadRequest extends Error {}

/** Opaque keyset cursor: the sort key values of the last row on the page. */
export function encodeCursor(values: (string | number)[]): string {
  return Buffer.from(JSON.stringify(values)).toString('base64url');
}

export function decodeCursor(cursor: string | undefined, arity: number): (string | number)[] | null {
  if (!cursor) return null;
  try {
    const v = JSON.parse(Buffer.from(cursor, 'base64url').toString('utf8'));
    if (Array.isArray(v) && v.length === arity && v.every((x) => typeof x === 'string' || typeof x === 'number')) {
      return v;
    }
  } catch {
    // fall through
  }
  throw new BadRequest('invalid cursor');
}

export const PageQuery = z.object({
  limit: z.coerce.number().int().min(1).max(200).default(50),
  cursor: z.string().optional(),
});

/** Optional ISO timestamp query parameter. */
export const isoDate = z
  .string()
  .refine((s) => !Number.isNaN(Date.parse(s)), 'must be an ISO 8601 date')
  .transform((s) => new Date(s));

/** Empty strings in a query mean "not set". */
export function clean(q: unknown): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries((q ?? {}) as Record<string, unknown>)) {
    if (v !== '' && v !== undefined) out[k] = v;
  }
  return out;
}

export function parseQuery<T extends z.ZodTypeAny>(schema: T, q: unknown): z.infer<T> {
  const r = schema.safeParse(clean(q));
  if (!r.success) {
    const issue = r.error.issues[0];
    throw new BadRequest(issue ? `${issue.path.join('.') || 'query'}: ${issue.message}` : 'bad query');
  }
  return r.data;
}

/** Fetches limit+1 rows; returns the page and the cursor for the next one. */
export function page<T>(rows: T[], limit: number, key: (row: T) => (string | number)[]) {
  const more = rows.length > limit;
  const items = more ? rows.slice(0, limit) : rows;
  const last = items[items.length - 1];
  return { items, next_cursor: more && last ? encodeCursor(key(last)) : null };
}

export const iso = (d: Date | string | null | undefined) =>
  d == null ? null : d instanceof Date ? d.toISOString() : new Date(d).toISOString();
