package ISUCON8::Portal::Model::Common;

use strict;
use warnings;
use feature 'state';
use parent 'ISUCON8::Portal::Model';

use ISUCON8::Portal::Exception;
use ISUCON8::Portal::Constants::Common;

use Mouse;

__PACKAGE__->meta->make_immutable;

no Mouse;

sub is_during_the_contest {
    my ($self, $params) = @_;
    my $start_at  = $params->{start_at};
    my $finish_at = $params->{finish_at};
    my $now       = time;

    return $start_at <= $now && $finish_at >= $now ? 1 : 0;
}

1;
