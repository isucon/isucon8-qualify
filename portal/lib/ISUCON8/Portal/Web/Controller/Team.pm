package ISUCON8::Portal::Web::Controller::Team;
use strict;
use warnings;
use feature 'state';

sub get_login {
    my ($self, $c) = @_;
    if ($c->session->get('team')) {
        return $c->redirect('/dashbord');
    }

    return $c->render('login.tx');
}

sub post_login {
    my ($self, $c) = @_;
    state $rule = $c->make_validator(
        id       => { isa => 'Str' },
        password => { isa => 'Str' },
    );

    my $params = $c->validate($rule, $c->req->body_parameters->mixed);
    unless ($params) {
        $c->log->warnf('validate error: %s', $rule->error->{message});
        $c->fillin_form($c->req);
        return $c->render('login.tx', {
            is_error => 1,
        });
    }

    my $team = $c->model('Team')->find_team($params);
    unless ($team) {
        $c->log->warnf('team login failed (id: %s)', $params->{id});
        $c->fillin_form($c->req);
        return $c->render('login.tx', {
            is_error => 1,
        });
    }

    $c->session->set(team => { id => $team->{id} });
    return $c->redirect('/');
}

sub get_logout {
    my ($self, $c) = @_;
    $c->session->expire;
    return $c->redirect('/login');
}

1;
