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

1;
