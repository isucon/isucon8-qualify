package ISUCON8::Portal::Web::Controller::Bench;
use strict;
use warnings;
use feature 'state';
use HTTP::Status qw(:constants);
use JSON;
use File::Slurp qw(read_file);

sub get_job {
    my ($self, $c) = @_;
    state $rule = $c->make_validator(
        hostname => { isa => 'Str' },
    );

    my $params = $c->validate($rule, $c->req->query_parameters->mixed);
    unless ($params) {
        return $c->create_response(
            HTTP_BAD_REQUEST,
            ['Content-Type', 'text/plain'],
            ['Invalid Params'],
        );
    }

    # 適当な daemon を作るのがめんどかったので定期的に叩かれるここでやる
    $c->model('Bench')->abort_timeout_job;

    my $job = $c->model('Bench')->dequeue_job($params);
    unless ($job) {
        return $c->create_response(HTTP_NO_CONTENT);
    }

    return $c->render_json($job);
}

sub post_job_result {
    my ($self, $c) = @_;
    state $rule = $c->make_validator(
        job_id     => { isa => 'Str' },
        is_aborted => { isa => 'Str', optional => 1, default => 0 },
    );

    my $params = $c->validate($rule, $c->req->query_parameters->mixed);
    unless ($params) {
        return $c->create_response(
            HTTP_BAD_REQUEST,
            ['Content-Type', 'text/plain'],
            ['Invalid Params'],
        );
    }

    my $job_id     = $params->{job_id};
    my $is_aborted = $params->{is_aborted} ? 1 : 0;

    my $result_json;
    if ($is_aborted) {
        $result_json = { reason => 'aborted' };
    } else {
        my $result_file = $c->req->upload('result');
        unless ($result_file) {
            return $c->create_response(
                HTTP_BAD_REQUEST,
                ['Content-Type', 'text/plain'],
                ['result json must be specified.'],
            );
        }

        $result_json = eval {
            $c->json->decode(scalar read_file $result_file->path);
        };
        if (my $e = $@) {
            $is_aborted  = 1;
            $result_json = { reason => 'Failed to decode result json' };
            $c->log->warnf('Cannot parse result json (job_id: %s)', $job_id);
        }
    }

    my $log_file = $c->req->upload('log');
    my $log      = $log_file ? read_file($log_file->path) : '';

    if ($is_aborted) {
        $c->model('Bench')->abort_job($job_id, $result_json, $log);
    }
    else {
        $c->model('Bench')->done_job($job_id, $result_json, $log);
    }

    return $c->render_json({ success => JSON::true });
}

1;
