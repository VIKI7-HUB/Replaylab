// One JSON object per line, so hosted log viewers can filter on fields.
type Fields = Record<string, unknown>;

function write(level: 'info' | 'warn' | 'error', msg: string, fields?: Fields) {
  const line = JSON.stringify({ t: new Date().toISOString(), level, msg, ...fields });
  if (level === 'error') process.stderr.write(line + '\n');
  else process.stdout.write(line + '\n');
}

export const log = {
  info: (msg: string, fields?: Fields) => write('info', msg, fields),
  warn: (msg: string, fields?: Fields) => write('warn', msg, fields),
  error: (msg: string, fields?: Fields) => write('error', msg, fields),
};

export function errMessage(err: unknown): string {
  if (err instanceof AggregateError && err.errors.length) return errMessage(err.errors[0]);
  if (err instanceof Error) {
    const code = (err as { code?: string }).code;
    const cause = (err as { cause?: { code?: string; message?: string } }).cause;
    const detail = cause ? ` (${cause.code ?? cause.message ?? ''})` : '';
    return (err.message || code || err.name) + detail;
  }
  return String(err);
}
