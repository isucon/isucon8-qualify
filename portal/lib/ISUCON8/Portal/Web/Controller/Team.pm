package ISUCON8::Portal::Web::Controller::Team;
use strict;
use warnings;
use feature 'state';

sub get_login {
    my ($self, $c) = @_;
    if ($c->session->get('team')) {
        return $c->redirect('/dashboard');
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

sub get_dashboard {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $team        = $model->get_team({ id => $team_id });
    my $servers     = $model->get_servers({ group_id => $team->{group_id} });
    my $score       = $model->get_latest_score({ team_id => $team_id });
    my $top_teams   = $model->get_team_scores({ limit => 10 });
    my $recent_jobs = $model->get_tema_jobs({ team_id => $team_id, limit => 10 });

    my ($target_server) = grep { $_->{is_target_host} } @$servers;

    return $c->render('dashboard.tx', {
        team          => $team,
        servers       => $servers,
        target_server => $target_server,
        score         => $score,
        top_teams     => $top_teams,
        recent_jobs   => $recent_jobs,
    });
}

1;
