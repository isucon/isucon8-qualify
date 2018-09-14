package ISUCON8::Portal::Exception;

use strict;
use warnings;
use parent 'Class::Accessor::Fast';
use overload (
    q|""| => \&to_string,
);

use JSON::XS;

my $_JSON = JSON::XS->new->utf8->canonical;

__PACKAGE__->mk_accessors(qw/code message data/);

sub throw {
    my ($class, %args) = @_;
    my $self = $class->new(\%args);
    if (ref $args{logger} eq 'CODE') {
        $args{logger}->($self->to_string);
    }
    die $self;
}

sub rethrow {
    my $self = shift;
    die $self;
}

sub to_string {
    my $self = shift;
    sprintf '%d: %s',
        $self->{code},
        join ' | ',
            $self->{message} ? $self->{message} : (),
            $self->{data}    ? (ref $self->{data} ? $_JSON->encode($self->{data}) : $self->{data}) : (),
    ;
}

1;
