package ISUCON8::Portal::Web::Controller::Root;
use strict;
use warnings;
use feature 'state';

sub get_index {
    my ($self, $c) = @_;

    my $contest_period = $c->config->{contest_period};
    unless ($c->model('Common')->is_during_the_contest($contest_period)) {
        return $c->render('landing.tx');
    }

    return $c->session->get('team') ? $c->redirect('/dashboard') : $c->redirect('/login');
}

1;
