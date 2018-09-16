package ISUCON8::Portal::Web::Dispatcher;
use strict;
use warnings;
use utf8;
use Amon2::Web::Dispatcher::RouterBoom;

use Module::Find qw(useall);
use ISUCON8::Portal::Web::Controller;

base 'ISUCON8::Portal::Web::Controller';
useall 'ISUCON8::Portal::Web::Controller';

get  '/'                => 'Root#get_index';
get  '/login'           => 'Team#get_login';
post '/login'           => 'Team#post_login';
get  '/logout'          => 'Team#get_logout';
get  '/dashboard'       => 'Team#get_dashboard';
get  '/jobs'            => 'Team#get_jobs';
get  '/jobs/{job_id}'   => 'Team#get_job_detail';
get  '/scores'          => 'Team#get_scores';
get  '/servers'         => 'Team#get_servers';

post '/api/job/enqueue'   => 'API#enqueue_job';
post '/api/job/cancel'    => 'API#cancel_job';
post '/api/target/change' => 'API#change_target';

get  '/admin'                 => 'Admin#get_index';
get  '/admin/dashboard'       => 'Admin#get_dashboard';
get  '/admin/login'           => 'Admin#get_login';
post '/admin/login'           => 'Admin#post_login';
get  '/admin/logout'          => 'Admin#get_logout';
get  '/admin/information'     => 'Admin#get_information';
post '/admin/information'     => 'Admin#post_information';
get  '/admin/jobs'            => 'Admin#get_jobs';
get  '/admin/jobs/{job_id}'   => 'Admin#get_job_detail';
get  '/admin/scores'          => 'Admin#get_scores';
get  '/admin/servers'         => 'Admin#get_servers';
get  '/admin/teams'           => 'Admin#get_teams';
get  '/admin/teams/{team_id}' => 'Admin#get_team_edit';
post '/admin/teams/{team_id}' => 'Admin#post_team_edit';

get  '/admin/enqueue'     => 'Admin#get_enqueue';
post '/admin/enqueue'     => 'Admin#post_enqueue';
get  '/admin/enqueue_all' => 'Admin#get_enqueue_all';
post '/admin/enqueue_all' => 'Admin#post_enqueue_all';

get  '/bench/job'        => 'Bench#get_job';
post '/bench/job/result' => 'Bench#post_job_result';

sub handle_exception {
    my ($class, $c, $e) = @_;
    my $env = $c->request->env;
    $c->log->critf(
        '%s %s: %s',
        $env->{REQUEST_METHOD}, $env->{PATH_INFO}, $e,
    );
    return $c->res_500;
}

1;
