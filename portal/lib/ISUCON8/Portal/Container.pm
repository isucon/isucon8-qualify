package ISUCON8::Portal::Container;

use strict;
use warnings;
use feature 'state';
use parent 'Micro::Container';

use SQL::Format;
use JSON::XS;
use Module::Find qw(useall);

__PACKAGE__->register(
    'JSON' => sub {
        JSON::XS->new->utf8->canonical;
    },
    'SQL' => sub {
        SQL::Format->new(driver => 'mysql');
    },
);

for my $model (useall 'ISUCON8::Portal::Model') {
    __PACKAGE__->register(
        $model => sub {
            my $c = shift;
            $model->new(container => $c);
        },
    );
}

sub set_config {
    my ($self, $config) = @_;
    $self->{config} = $config;
}

sub config {
    shift->{config};
}

1;
