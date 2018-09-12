package ISUCON8::Portal::Web::Plugin::Session;
use strict;
use warnings;
use utf8;

use Amon2::Util;
use HTTP::Session2::ServerStore;
use ISUCON8::Portal::SessionStore;

sub init {
    my ($class, $c) = @_;

    # Validate XSRF Token.
    $c->add_trigger(
        BEFORE_DISPATCH => sub {
            my ($c) = @_;
            my $path   = $c->req->path;
            my $method = $c->req->method;

            # benchmaker は session 不要
            if ($path =~ m|^/bench|) {
                return;
            }

            if ($path =~ m|^/admin|) {
                if ($path ne '/admin/login') {
                    unless ($c->session->get('admin')) {
                        $c->session->expire;
                        return $c->redirect('/admin/login');
                    }
                }
            }
            else {
                my $contest_period = $c->config->{contest_period};
                unless ($c->model('Common')->is_during_the_contest($contest_period)) {
                    unless ($path eq '/') {
                        return $c->redirect('/');
                    }
                }
                else {
                    if ($path ne '/login') {
                        unless ($c->session->get('team')) {
                            $c->session->expire;
                            return $c->redirect('/login');
                        }
                    }
                }
            }

            if ($c->req->method ne 'GET' && $c->req->method ne 'HEAD') {
                my $token = $c->req->header('X-XSRF-TOKEN') || $c->req->param('XSRF-TOKEN');
                unless ($c->session->validate_xsrf_token($token)) {
                    return $c->create_simple_status_page(
                        403, 'XSRF detected.'
                    );
                }
            }
            return;
        },
    );

    Amon2::Util::add_method($c, 'session', \&_session);

    # Inject cookie header after dispatching.
    $c->add_trigger(
        AFTER_DISPATCH => sub {
            my ( $c, $res ) = @_;
            if ($c->{session} && $res->can('cookies')) {
                $c->{session}->finalize_plack_response($res);
            }
            return;
        },
    );
}

sub _session {
    my $self = shift;

    if (!exists $self->{session}) {
        $self->{session} = HTTP::Session2::ServerStore->new(
            env       => $self->req->env,
            secret    => 'RTU1ZE7Y_l3yy3VoqJ2v5ssoYjJ0WJpU',
            get_store => sub {
                ISUCON8::Portal::SessionStore->new(context => $self);
            },
        );
    }
    return $self->{session};
}

1;
__END__

=head1 DESCRIPTION

This module manages session for ISUCON8::Portal.

