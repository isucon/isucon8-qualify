package ISUCON8::Portal::Constants::Common;

use strict;
use warnings;
use parent 'ISUCON8::Portal::Constants';

__PACKAGE__->constants(
    ERROR_BAD_REQUEST         => 400,
    ERROR_UNAUTHORIZED        => 401,
    ERROR_NOT_FOUND           => 404,
    ERROR_FORBIDDEN           => 403,
    ERROR_CONFLICT            => 409,
    ERROR_TOO_MANY_REQUESTS   => 429,
    ERROR_INTERNAL_ERROR      => 500,
    ERROR_SERVICE_UNAVAILABLE => 503,
);

__PACKAGE__->constants(
    TEAM_STATE_ACTIVED => 'actived',
    TEAM_STATE_BANNED  => 'banned',
);

__PACKAGE__->constants(
    JOB_QUEUE_STATE_WAITING  => 'waiting',
    JOB_QUEUE_STATE_RUNNING  => 'running',
    JOB_QUEUE_STATE_DONE     => 'done',
    JOB_QUEUE_STATE_ABORTED  => 'aborted',
    JOB_QUEUE_STATE_CANCELED => 'canceled',
);

__PACKAGE__->constants(
    BENCHMARK_MAX_CONCURRENCY => 3,
);

__PACKAGE__->constants(
    JOB_RESULT_PASS    => 'pass',
    JOB_RESULT_FAIL    => 'fail',
    JOB_RESULT_UNKNOWN => 'unknown',
);

1;
