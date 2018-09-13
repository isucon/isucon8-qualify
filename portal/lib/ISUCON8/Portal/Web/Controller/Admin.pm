package ISUCON8::Portal::Web::Controller::Admin;
use strict;
use warnings;
use feature 'state';

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

sub get_index {
    my ($self, $c) = @_;
    return $c->render_admin('admin/index.tx');
}

1;
