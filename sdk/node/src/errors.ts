/** Base error for all MuninnDB client errors. */
export class MuninnError extends Error {
  public readonly statusCode: number | undefined;
  public readonly body: unknown;

  constructor(message: string, statusCode?: number, body?: unknown) {
    super(message);
    this.name = "MuninnError";
    this.statusCode = statusCode;
    this.body = body;
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/** Thrown when the server returns 401 Unauthorized. */
export class MuninnAuthError extends MuninnError {
  constructor(message = "Authentication failed", body?: unknown) {
    super(message, 401, body);
    this.name = "MuninnAuthError";
  }
}

/** Thrown when the server returns 404 Not Found. */
export class MuninnNotFoundError extends MuninnError {
  constructor(message = "Resource not found", body?: unknown) {
    super(message, 404, body);
    this.name = "MuninnNotFoundError";
  }
}

/** Thrown when the server returns 409 Conflict. */
export class MuninnConflictError extends MuninnError {
  constructor(message = "Conflict", body?: unknown) {
    super(message, 409, body);
    this.name = "MuninnConflictError";
  }
}

/** Thrown when the server returns a 5xx status code. */
export class MuninnServerError extends MuninnError {
  constructor(message = "Server error", statusCode = 500, body?: unknown) {
    super(message, statusCode, body);
    this.name = "MuninnServerError";
  }
}

/** Thrown when a connection to the server cannot be established. */
export class MuninnConnectionError extends MuninnError {
  public readonly cause: unknown;

  constructor(message = "Connection failed", cause?: unknown) {
    super(message);
    this.name = "MuninnConnectionError";
    this.cause = cause;
  }
}

/** Thrown when a request exceeds the configured timeout. */
export class MuninnTimeoutError extends MuninnError {
  constructor(message = "Request timed out") {
    super(message);
    this.name = "MuninnTimeoutError";
  }
}

/**
 * Maps an HTTP status code and response body to the appropriate error class.
 */
export function errorFromStatus(
  status: number,
  body: unknown,
  fallbackMessage?: string,
): MuninnError {
  const msg =
    (body && typeof body === "object" && "error" in body
      ? String((body as Record<string, unknown>).error)
      : undefined) ?? fallbackMessage;

  switch (status) {
    case 401:
      return new MuninnAuthError(msg, body);
    case 404:
      return new MuninnNotFoundError(msg, body);
    case 409:
      return new MuninnConflictError(msg, body);
    default:
      if (status >= 500) {
        return new MuninnServerError(msg, status, body);
      }
      return new MuninnError(msg ?? `HTTP ${status}`, status, body);
  }
}
