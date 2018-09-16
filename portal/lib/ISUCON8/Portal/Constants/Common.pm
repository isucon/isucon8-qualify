package ISUCON8::Portal::Constants::Common;

use strict;
use warnings;
use parent 'ISUCON8::Portal::Constants';
use utf8;

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
    TEAM_STATE_ACTIVE  => 'active',
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
    BENCHMARK_MAX_CONCURRENCY => 1,
);

__PACKAGE__->constants(
    JOB_RESULT_PASS    => 'pass',
    JOB_RESULT_FAIL    => 'fail',
    JOB_RESULT_UNKNOWN => 'unknown',
);

__PACKAGE__->constants(
    TEAM_CATEGORY_GENERAL_ONE   => 'general_one',
    TEAM_CATEGORY_GENERAL_TWO   => 'general_two',
    TEAM_CATEGORY_GENERAL_THREE => 'general_three',
    TEAM_CATEGORY_STUDENT_ONE   => 'student_one',
    TEAM_CATEGORY_STUDENT_TWO   => 'student_two',
    TEAM_CATEGORY_STUDENT_THREE => 'student_three',
);

__PACKAGE__->constants(
    TEAM_CATEGORY_TO_DISPLAY_NAME_MAP => {
        TEAM_CATEGORY_GENERAL_ONE()   => '1人',
        TEAM_CATEGORY_GENERAL_TWO()   => '2人',
        TEAM_CATEGORY_GENERAL_THREE() => '3人',
        TEAM_CATEGORY_STUDENT_ONE()   => '学生1人',
        TEAM_CATEGORY_STUDENT_TWO()   => '学生2人',
        TEAM_CATEGORY_STUDENT_THREE() => '学生3人',
    },
);

1;
