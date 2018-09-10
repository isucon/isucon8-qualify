package ISUCON8::Portal::Web::Controller::Team;
use strict;
use warnings;
use feature 'state';

sub get_login {
    my ($self, $c) = @_;
    $c->render('login.tx');
}

sub post_logint {
    my ($self, $c) = @_;
}

1;
