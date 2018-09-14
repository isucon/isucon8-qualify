package ISUCON8::Portal::Web;
use strict;
use warnings;
use utf8;
use feature qw/state/;
use parent qw/ISUCON8::Portal Amon2::Web/;
use File::Spec;

# dispatcher
use ISUCON8::Portal::Web::Dispatcher;
sub dispatch {
    return (ISUCON8::Portal::Web::Dispatcher->dispatch($_[0]) or die "response is not generated");
}

__PACKAGE__->mk_accessors(qw/stash/);

# load plugins
__PACKAGE__->load_plugins(
    'Web::FillInFormLite',
    'Web::JSON',
    '+ISUCON8::Portal::Web::Plugin::Session',
);

# setup view
use ISUCON8::Portal::Web::View;
{
    sub create_view {
        my $view = ISUCON8::Portal::Web::View->make_instance(__PACKAGE__);
        no warnings 'redefine';
        *ISUCON8::Portal::Web::create_view = sub { $view }; # Class cache.
        $view
    }
}

# clear stash
__PACKAGE__->add_trigger('BEFORE_DISPATCH', sub {
    state $srand_called = do { srand() };
    $_[0]->stash(+{});
    $_[0]->session(+{});
});

# for your security
__PACKAGE__->add_trigger(
    AFTER_DISPATCH => sub {
        my ( $c, $res ) = @_;

        # http://blogs.msdn.com/b/ie/archive/2008/07/02/ie8-security-part-v-comprehensive-protection.aspx
        $res->header( 'X-Content-Type-Options' => 'nosniff' );

        # http://blog.mozilla.com/security/2010/09/08/x-frame-options/
        $res->header( 'X-Frame-Options' => 'DENY' );

        # Cache control.
        $res->header( 'Cache-Control' => 'private' );
    },
);

__PACKAGE__->add_trigger(
    AFTER_DISPATCH => sub {
        my ($c, $res) = @_;
        $c->db->disconnect;
    },
);

sub to_app {
    my ($class) = @_;
    $class->_load_config;
    $class->SUPER::to_app;
}

sub team_id {
    my ($c) = @_;
    my $team = $c->session->get('team');
    return $team ? $team->{id} : 0;
}

sub contest_start_at {
    my ($c) = @_;
    state $start_at = $c->config->{contest_period}{start_at};
}

sub contest_finish_at {
    my ($c) = @_;
    state $finish_at = $c->config->{contest_period}{finish_at};
}

sub last_spurt_time {
    my ($c) = @_;
    state $last_spurt_time = $c->contest_finish_at - 60 * 60;
}

# のこり1時間に迫ったら true
sub is_last_spurt {
    my ($c) = @_;
    return time > $c->last_spurt_time ? 1 : 0;
}

sub is_started {
    my ($c) = @_;
    return time > $c->contest_finish_at ? 1 : 0;
}

sub is_finished {
    my ($c) = @_;
    return time > $c->contest_finish_at ? 1 : 0;
}

sub is_during_the_contest {
    my ($c) = @_;

    my $now = time;
    return $c->contest_start_at <= $now && $c->contest_finish_at >= $now ? 1 : 0;
}

sub render {
    my ($c, $tmpl, $params) = @_;
    $params->{config} = $c->config;

    my $prev_page = $c->req->query_parameters->mixed->{_prev};
    if ($prev_page) {
        $params->{_prev} = URI->new($prev_page)->path_query;
    }

    $c->SUPER::render($tmpl, $params);
}

sub redirect {
    my ($c, $location, $params) = @_;
    if (!URI->new($location)->query_param('_prev') && !exists $params->{_prev}) {
        my $prev_page = $c->req->query_parameters->mixed->{_prev};
        if ($prev_page) {
            $params->{_prev} = URI->new($prev_page)->path_query;
        }
    }

    $c->SUPER::redirect($location, $params);
}

sub redirect_myself {
    my ($c, $params) = @_;
    my $req = $c->req;
    my $u = URI->new('?'.$req->env->{QUERY_STRING});
    $u->query_param_delete($_) for qw/update create/;
    local $req->env->{QUERY_STRING} = substr $u->as_string, 1;
    $c->redirect($req->uri_with($params || {}));
}

sub uri_with {
    shift->req->uri_with(@_);
}

sub current_uri {
    shift->req->uri;
}

sub current_path {
    shift->current_uri->path;
}

sub current_path_query {
    shift->current_uri->path_query;
}

sub query_params {
    shift->req->query_parameters->mixed;
}

sub body_params {
    shift->req->body_parameters->mixed;
}

sub find_params {
    shift->query_params->{ $_[0 ] };
}

sub res_400 {
    my $c = shift;
    return $c->create_simple_status_page(400, 'Bad Request');
}

sub res_403 {
    my $c = shift;
    return $c->create_simple_status_page(403, 'Forbidden');
}

sub res_404 {
    my $c = shift;
    return $c->create_simple_status_page(404, 'Not Found');
}

sub health_check {
    my ($class) = @_;

    return [
        200,
        [
            'Content-Type'   => 'text/plain',
            'Content-Length' => 2,
        ],
        [ 'OK' ],
    ];
}

1;
