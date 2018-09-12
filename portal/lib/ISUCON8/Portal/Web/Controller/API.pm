package ISUCON8::Portal::Web::Controller::API;
use strict;
use warnings;
use feature 'state';
use JSON;

sub enqueue_job {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    unless ($team_id) {
        $c->res_400;
    }

    my ($is_success, $err) = $c->model('Bench')->enqueue_job({ team_id => $team_id });

    if ($is_success) {
        return $c->render_json({ success => JSON::true });
    }
    else {
        return $c->render_json({ success => JSON::false, error => $err });
    }
}

sub cancel_job {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    unless ($team_id) {
        $c->res_400;
    }
}

sub get_dashbord {
    my ($self, $c) = @_;
    my $team_id = $c->team_id;
    my $model   = $c->model('Team');

    my $team        = $model->get_team({ id => $team_id });
    my $servers     = $model->get_servers({ group_id => $team->{group_id} });
    my $score       = $model->get_latest_score({ team_id => $team_id });
    my $top_teams   = $model->get_team_scores({ limit => 10 });
    my $recent_jobs = $model->get_tema_jobs({ team_id => $team_id, limit => 10 });

    my ($target_server) = grep { $_->{is_target_host} } @$servers;

    return $c->render('dashbord.tx', {
        team          => $team,
        servers       => $servers,
        target_server => $target_server,
        score         => $score,
        top_teams     => $top_teams,
        recent_jobs   => $recent_jobs,
    });
}

1;
