package ISUCON8::Portal::Web::Controller::Root;
use strict;
use warnings;
use feature 'state';

sub get_index {
    my ($self, $c) = @_;
    unless ($c->is_during_the_contest) {
        return $c->render('landing.tx');
    }

    return $c->session->get('team') ? $c->redirect('/dashboard') : $c->redirect('/login');
}

1;
