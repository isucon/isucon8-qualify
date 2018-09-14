package ISUCON8::Portal::Web::Controller::Admin;
use strict;
use warnings;
use feature 'state';

sub get_index {
    my ($self, $c) = @_;
    return $c->redirect('/admin/dashboard');
}

sub get_login {
    my ($self, $c) = @_;
    if ($c->session->get('admin')) {
        return $c->redirect('/admin');
    }

    return $c->render_admin('admin/login.tx');
}

sub post_login {
    my ($self, $c) = @_;
    state $rule = $c->make_validator(
        name     => { isa => 'Str' },
        password => { isa => 'Str' },
    );

    my $params = $c->validate($rule, $c->req->body_parameters->mixed);
    unless ($params) {
        $c->log->warnf('validate error: %s', $rule->error->{message});
        $c->fillin_form($c->req);
        return $c->render_admin('admin/login.tx', {
            is_error => 1,
        });
    }

    my $user = $c->model('Admin')->find_user($params);
    unless ($user) {
        $c->log->warnf('admin login failed (name: %s)', $params->{name});
        $c->fillin_form($c->req);
        return $c->render_admin('admin/login.tx', {
            is_error => 1,
        });
    }

    $c->session->set(admin => $user->{name});
    return $c->redirect('/admin');
}

sub get_logout {
    my ($self, $c) = @_;
    $c->session->remove('admin');
    $c->redirect('/admin/login');
}

sub get_dashboard {
    my ($self, $c) = @_;
    my $model = $c->model('Admin');

    my $processiong_jobs = $model->get_processing_jobs;
    my $info             = $model->get_information;
    my $scores           = $model->get_team_scores({ limit => 30 });
    my $chart_data       = $model->get_chart_data({
        team_id => 0,
        limit   => 30,
    });
    return $c->render_admin('admin/index.tx', {
        page             => 'dashboard',
        info             => $info,
        processiong_jobs => $processiong_jobs,
        scores           => $scores,
        chart_data       => $chart_data,
    });
}

sub get_jobs {
    my ($self, $c) = @_;
    my $model = $c->model('Admin');

    my $all_jobs = $model->get_all_jobs;
    my $info     = $model->get_information;
    return $c->render_admin('admin/jobs.tx', {
        page     => 'jobs',
        info     => $info,
        all_jobs => $all_jobs,
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

    my $model = $c->model('Admin');
    my $job   = $model->get_job({ job_id => $params->{job_id} });
    my $info  = $model->get_information;
    return $c->render_admin('admin/job_detail.tx', {
        page => 'jobs',
        info => $info,
        job  => $job,
    });
}

sub get_information {
    my ($self, $c) = @_;

    my $model = $c->model('Admin');
    my $info  = $model->get_information();
    return $c->render_admin('admin/information.tx', {
        page => 'information',
        info => $info,
    });
}

sub post_information {
    my ($self, $c) = @_;
    state $rule = $c->make_validator(
        message => { isa => 'Str' },
    );
    my $params = $c->validate($rule, $c->req->body_parameters->mixed);
    unless ($params) {
        $c->log->warnf('validate error: %s', $rule->error->{message});
        return $c->res_400;
    }

    my $model = $c->model('Admin');
    $model->update_information({ message => $params->{message} });

    my $info = $model->get_information;
    return $c->render_admin('admin/information.tx', {
        page => 'information',
        info => $info,
    });
}

sub get_scores {
    my ($self, $c) = @_;
    my $model = $c->model('Admin');

    my $info       = $model->get_information;
    my $scores     = $model->get_team_scores();
    my $chart_data = $model->get_chart_data({ team_id => 0 });
    return $c->render_admin('admin/scores.tx', {
        page       => 'scores',
        info       => $info,
        scores     => $scores,
        chart_data => $chart_data,
    });
}

sub get_servers {
    my ($self, $c) = @_;
    my $model = $c->model('Admin');

    my $info    = $model->get_information;
    my $servers = $model->get_servers;
    return $c->render_admin('admin/servers.tx', {
        page    => 'servers',
        info    => $info,
        servers => $servers,
    });
}

1;
