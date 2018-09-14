package ISUCON8::Portal::Web::Controller::Team;
use strict;
use warnings;
use feature 'state';
use List::Util qw(uniq);

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

    my $info        = $model->get_information;
    my $team        = $model->get_team({ id => $team_id });
    my $servers     = $model->get_servers({ group_id => $team->{group_id} });
    my $score       = $model->get_latest_score({ team_id => $team_id });
    my $top_teams   = $model->get_team_scores({ limit => 30 });
    my $recent_jobs = $model->get_team_jobs({ team_id => $team_id, limit => 10 });

    my $chart_data = $model->get_chart_data({
        team_id         => $team_id,
        is_last_spurt   => $c->is_last_spurt,
        last_spurt_time => $c->last_spurt_time,
        limit           => 30,
    });
    my ($target_server) = grep { $_->{is_target_host} } @$servers;

    return $c->render('dashboard.tx', {
        page          => 'dashboard',
        info          => $info,
        team          => $team,
        servers       => $servers,
        target_server => $target_server,
        score         => $score,
        top_teams     => $top_teams,
        recent_jobs   => $recent_jobs,
        chart_data    => $chart_data,
    });
}

sub get_jobs {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $info  = $model->get_information;
    my $team  = $model->get_team({ id => $team_id });
    my $score = $model->get_latest_score({ team_id => $team_id });
    my $jobs  = $model->get_team_jobs({ team_id => $team_id });

    return $c->render('jobs.tx', {
        page  => 'jobs',
        info  => $info,
        team  => $team,
        score => $score,
        jobs  => $jobs,
    });
}

sub get_job_detail {
    my ($self, $c, $captured) = @_;
    state $rule = $c->make_validator(
        job_id => { isa => 'Str' },
    );

    my $params = $c->validate($rule, $captured);
    unless ($params) {
        $c->log->warnf('validate error: %s', $rule->error->{message});
        return $c->res_404;
    }

    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $info  = $model->get_information;
    my $team  = $model->get_team({ id => $team_id });
    my $score = $model->get_latest_score({ team_id => $team_id });
    my $job   = $model->get_team_job({
        team_id => $team_id,
        job_id  => $params->{job_id},
    });

    unless ($job) {
        return $c->res_404;
    }

    return $c->render('job_detail.tx', {
        page  => 'jobs',
        info  => $info,
        team  => $team,
        score => $score,
        job   => $job,
    });
}

sub get_servers {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $info    = $model->get_information;
    my $team    = $model->get_team({ id => $team_id });
    my $score   = $model->get_latest_score({ team_id => $team_id });
    my $servers = $model->get_servers({ group_id => $team->{group_id} });

    unless ($servers) {
        return $c->res_404;
    }

    return $c->render('servers.tx', {
        page    => 'servers',
        info    => $info,
        team    => $team,
        score   => $score,
        servers => $servers,
    });
}

sub get_scores {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $info   = $model->get_information;
    my $team   = $model->get_team({ id => $team_id });
    my $score  = $model->get_latest_score({ team_id => $team_id });
    my $scores = $model->get_team_scores({});

    return $c->render('scores.tx', {
        page   => 'scores',
        info   => $info,
        team   => $team,
        score  => $score,
        scores => $scores,
    });
}

1;
