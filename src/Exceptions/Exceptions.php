<?php

declare(strict_types=1);

namespace MuninnDB\Exceptions;

class MuninnException extends \RuntimeException
{
    public function __construct(
        string $message = '',
        int $code = 0,
        ?\Throwable $previous = null,
        public readonly ?int $httpStatus = null,
        public readonly ?string $responseBody = null,
    ) {
        parent::__construct($message, $code, $previous);
    }
}

class AuthException extends MuninnException
{
    public function __construct(string $message = 'Authentication failed', ?\Throwable $previous = null)
    {
        parent::__construct($message, 401, $previous, httpStatus: 401);
    }
}

class NotFoundException extends MuninnException
{
    public function __construct(string $message = 'Resource not found', ?\Throwable $previous = null)
    {
        parent::__construct($message, 404, $previous, httpStatus: 404);
    }
}

class ConflictException extends MuninnException
{
    public function __construct(string $message = 'Conflict', ?string $responseBody = null, ?\Throwable $previous = null)
    {
        parent::__construct($message, 409, $previous, httpStatus: 409, responseBody: $responseBody);
    }
}

class ValidationException extends MuninnException
{
    public function __construct(string $message = 'Validation error', ?string $responseBody = null, ?\Throwable $previous = null)
    {
        parent::__construct($message, 400, $previous, httpStatus: 400, responseBody: $responseBody);
    }
}

class ServerException extends MuninnException
{
    public function __construct(string $message = 'Server error', int $httpStatus = 500, ?string $responseBody = null, ?\Throwable $previous = null)
    {
        parent::__construct($message, $httpStatus, $previous, httpStatus: $httpStatus, responseBody: $responseBody);
    }
}

class ConnectionException extends MuninnException
{
    public function __construct(string $message = 'Connection failed', int $curlError = 0, ?\Throwable $previous = null)
    {
        parent::__construct($message, $curlError, $previous);
    }
}

class TimeoutException extends MuninnException
{
    public function __construct(string $message = 'Request timed out', ?\Throwable $previous = null)
    {
        parent::__construct($message, 0, $previous);
    }
}
