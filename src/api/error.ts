export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    Object.setPrototypeOf(this, ApiError.prototype);
  }
}

export function normalizeError(err: unknown, service: string, url: string): ApiError {
  if (err instanceof ApiError) return err;
  if (err instanceof TypeError && err.message.toLowerCase().includes('fetch')) {
    return new ApiError(0, `Can't reach ${service} at ${url}`);
  }
  if (err instanceof DOMException && err.name === 'AbortError') {
    return new ApiError(0, 'Request aborted');
  }
  if (err instanceof Error) {
    return new ApiError(0, err.message);
  }
  return new ApiError(0, `Can't reach ${service} at ${url}`);
}

