<?php

declare(strict_types=1);

namespace MuninnDB;

use MuninnDB\Exceptions\ConnectionException;
use MuninnDB\Exceptions\TimeoutException;
use MuninnDB\Types\SseEvent;

/**
 * Server-Sent Events stream backed by a persistent curl connection.
 * Implements Iterator so callers can `foreach ($stream as $event)`.
 */
class SseStream implements \Iterator
{
    private \CurlHandle $ch;
    private string $buffer = '';

    /** @var SseEvent[] */
    private array $queue = [];
    private int $index = 0;
    private bool $closed = false;
    private ?SseEvent $current = null;

    public function __construct(
        private readonly string $url,
        private readonly string $token,
        private readonly float $timeout = 0,
    ) {
        $this->ch = curl_init();
        curl_setopt_array($this->ch, [
            CURLOPT_URL            => $this->url,
            CURLOPT_HTTPHEADER     => array_filter([
                'Accept: text/event-stream',
                'Cache-Control: no-cache',
                $this->token !== '' ? 'Authorization: Bearer ' . $this->token : null,
            ]),
            CURLOPT_WRITEFUNCTION  => $this->onData(...),
            CURLOPT_RETURNTRANSFER => false,
            CURLOPT_FOLLOWLOCATION => true,
            CURLOPT_TIMEOUT        => (int) $this->timeout,
            CURLOPT_CONNECTTIMEOUT => 5,
        ]);
    }

    public function close(): void
    {
        if (!$this->closed) {
            $this->closed = true;
            curl_close($this->ch);
        }
    }

    public function __destruct()
    {
        $this->close();
    }

    // ── Iterator implementation ──────────────────────────────

    public function current(): SseEvent
    {
        return $this->current;
    }

    public function key(): int
    {
        return $this->index;
    }

    public function next(): void
    {
        $this->index++;
        $this->current = $this->readNext();
    }

    public function rewind(): void
    {
        $this->index = 0;
        $this->current = $this->readNext();
    }

    public function valid(): bool
    {
        return $this->current !== null;
    }

    // ── Internal ─────────────────────────────────────────────

    /**
     * Curl write-function callback.  Buffers incoming chunks and parses SSE frames.
     */
    private function onData(\CurlHandle $ch, string $data): int
    {
        $this->buffer .= $data;
        $this->parseBuffer();
        return strlen($data);
    }

    private function parseBuffer(): void
    {
        while (($pos = strpos($this->buffer, "\n\n")) !== false) {
            $frame = substr($this->buffer, 0, $pos);
            $this->buffer = substr($this->buffer, $pos + 2);

            $event = 'message';
            $dataLines = [];

            foreach (explode("\n", $frame) as $line) {
                if (str_starts_with($line, 'event:')) {
                    $event = trim(substr($line, 6));
                } elseif (str_starts_with($line, 'data:')) {
                    $dataLines[] = trim(substr($line, 5));
                }
            }

            if ($dataLines === []) {
                continue;
            }

            $payload = implode("\n", $dataLines);
            $decoded = json_decode($payload, true);

            $this->queue[] = SseEvent::fromArray(
                data: is_array($decoded) ? $decoded : ['raw' => $payload],
                event: $event,
            );
        }
    }

    /**
     * Drains the queue or performs a blocking curl_multi read to get the next event.
     */
    private function readNext(): ?SseEvent
    {
        if ($this->queue !== []) {
            return array_shift($this->queue);
        }

        if ($this->closed) {
            return null;
        }

        $mh = curl_multi_init();
        curl_multi_add_handle($mh, $this->ch);

        do {
            $status = curl_multi_exec($mh, $running);

            if ($this->queue !== []) {
                curl_multi_remove_handle($mh, $this->ch);
                curl_multi_close($mh);
                return array_shift($this->queue);
            }

            if ($running > 0) {
                curl_multi_select($mh, 0.1);
            }
        } while ($running > 0 && $status === CURLM_OK);

        $error = curl_error($this->ch);
        $errno = curl_errno($this->ch);

        curl_multi_remove_handle($mh, $this->ch);
        curl_multi_close($mh);

        $this->closed = true;

        if ($this->queue !== []) {
            return array_shift($this->queue);
        }

        if ($errno === CURLE_OPERATION_TIMEDOUT) {
            throw new TimeoutException('SSE stream timed out');
        }

        if ($errno !== 0) {
            throw new ConnectionException("SSE connection error: $error", $errno);
        }

        return null;
    }
}
