import type { SseEvent } from "./types.js";

/**
 * Parse a single SSE text frame into an {@link SseEvent}.
 *
 * Handles the `event:`, `data:`, `id:`, and `retry:` fields per the
 * W3C Server-Sent Events specification.
 */
function parseFrame(raw: string): SseEvent | undefined {
  let event: string | undefined;
  let id: string | undefined;
  let retry: number | undefined;
  const dataLines: string[] = [];

  for (const line of raw.split("\n")) {
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    } else if (line.startsWith("event:")) {
      event = line.slice(6).trimStart();
    } else if (line.startsWith("id:")) {
      id = line.slice(3).trimStart();
    } else if (line.startsWith("retry:")) {
      const n = Number(line.slice(6).trimStart());
      if (!Number.isNaN(n)) retry = n;
    }
  }

  if (dataLines.length === 0) return undefined;

  const joined = dataLines.join("\n");
  let data: Record<string, unknown>;
  try {
    data = JSON.parse(joined) as Record<string, unknown>;
  } catch {
    data = { raw: joined };
  }

  return { event, data, id, retry };
}

/**
 * Consume a {@link ReadableStream<Uint8Array>} from a `fetch` response as an
 * async iterable of parsed {@link SseEvent} objects.
 *
 * The caller is responsible for aborting the underlying request when done.
 */
export async function* streamSse(
  body: ReadableStream<Uint8Array>,
): AsyncGenerator<SseEvent, void, undefined> {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    for (;;) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });

      // SSE frames are separated by double newlines.
      let boundary: number;
      while ((boundary = buffer.indexOf("\n\n")) !== -1) {
        const frame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + 2);

        const evt = parseFrame(frame);
        if (evt) yield evt;
      }
    }

    // Flush any trailing frame.
    if (buffer.trim().length > 0) {
      const evt = parseFrame(buffer);
      if (evt) yield evt;
    }
  } finally {
    reader.releaseLock();
  }
}
