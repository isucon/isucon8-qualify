package ISUCON8::Portal::Team;

use strict;
use warnings;
use parent 'Class::Accessor::Fast';

__PACKAGE__->mk_accessors(qw/id name state category/);

sub serialize {
    my $self = shift;
    return +{
        id       => $self->{id},
        name     => $self->{name},
        state    => $self->{state},
        category => $self->{category},
    };
}

1;
