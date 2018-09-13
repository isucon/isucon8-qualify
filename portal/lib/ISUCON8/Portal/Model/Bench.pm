package ISUCON8::Portal::Model::Bench;

use strict;
use warnings;
use feature 'state';
use parent 'ISUCON8::Portal::Model';

use ISUCON8::Portal::Exception;
use ISUCON8::Portal::Constants::Common;

use Mouse;

__PACKAGE__->meta->make_immutable;

no Mouse;

sub enqueue_job {
    my ($self, $params) = @_;
    my $team_id = $params->{team_id};

    my $is_success = 0;
    my $err        = undef;
    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                ['COUNT(id)'],
                {
                    team_id => $team_id,
                    state   => [ JOB_QUEUE_STATE_WAITING, JOB_QUEUE_STATE_RUNNING ],
                },
            );
            my ($rv) = $dbh->selectrow_array($stmt, undef, @bind);
            if ($rv > 0) {
                $err = 'Benchmark Job already exists!';
                return;
            }

            ($stmt, @bind) = $self->sql->select(
                { teams => 't' },
                [\'s.*'],
                {},
                {
                    join => {
                        table     => { 'servers' => 's' },
                        condition => ['group_id'],
                    },
                },
            );
            my $servers = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
            unless (@$servers) {
                $err = 'Oops! Your teams server is not found!?';
                return;
            }

            my ($benchmaker) = grep { $_->{is_bench_host}  } @$servers;
            my ($target)     = grep { $_->{is_target_host} } @$servers;
            ($stmt, @bind) = $self->sql->insert(
                'bench_queues',
                {
                    team_id        => $team_id,
                    node           => $benchmaker->{node},
                    bench_hostname => $benchmaker->{hostname},
                    target_ip      => $target->{bench_ip},
                    state          => JOB_QUEUE_STATE_WAITING,
                    created_at     => \'UNIX_TIMESTAMP()',
                    updated_at     => \'UNIX_TIMESTAMP()',
                },
            );
            $dbh->do($stmt, undef, @bind);

            $is_success = 1;
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return $is_success, $err;
}

sub dequeue_job {
    my ($self, $params) = @_;
    my $hostname = $params->{hostname};

    my $job;
    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                [qw/id target_ip node state/],
                {
                    bench_hostname => $hostname,
                    state          => [ JOB_QUEUE_STATE_WAITING, JOB_QUEUE_STATE_RUNNING ],
                },
                {
                    limit    => 1,
                    order_by => { -asc => 'created_at' },
                },
            );
            my $row = $dbh->selectrow_hashref($stmt, undef, @bind);
            return unless $row;
            return if $row->{state} eq JOB_QUEUE_STATE_RUNNING;

            # ホストサーバーの性能を使い切ってしまう可能性があるので
            # 同一ホストでの最大の並列数を超えないようにする
            ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                ['COUNT(*)'],
                {
                    node  => $row->{node},
                    state => JOB_QUEUE_STATE_RUNNING,
                },
            );
            my ($rc) = $dbh->selectrow_array($stmt, undef, @bind);
            return if $rc >= BENCHMARK_MAX_CONCURRENCY;

            ($stmt, @bind) = $self->sql->update(
                'bench_queues',
                {
                    state      => JOB_QUEUE_STATE_RUNNING,
                    updated_at => \'UNIX_TIMESTAMP()',
                },
                {
                    id => $row->{id},
                },
            );
            $dbh->do($stmt, undef, @bind);

            $job = $row;
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return $job;
}

sub done_job {
    my ($self, $job_id, $result_json, $log) = @_;

    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                ['team_id'],
                {
                    id    => $job_id,
                    state => JOB_QUEUE_STATE_RUNNING,
                },
            );
            my ($team_id) = $dbh->selectrow_array($stmt, undef, @bind);
            unless ($team_id) {
                ISUCON8::Portal::Exception->throw(
                    code    => ERROR_CONFLICT,
                    message => 'bench_queues cannot update',
                    logger  => sub { $self->log->warnf(@_) },
                );
            }

            ($stmt, @bind) = $self->sql->update(
                'bench_queues',
                {
                    state         => JOB_QUEUE_STATE_DONE,
                    result_score  => $result_json->{score},
                    result_status => $result_json->{pass} ? JOB_RESULT_PASS : JOB_RESULT_FAIL,
                    result_json   => $self->json->encode($result_json),
                    log_text      => $log,
                    updated_at    => \'UNIX_TIMESTAMP()',
                },
                {
                    id => $job_id,
                },
            );
            $dbh->do($stmt, undef, @bind);

            ($stmt, @bind) = $self->sql->insert_on_duplicate(
                'team_scores',
                {
                    team_id       => $team_id,
                    latest_score  => $result_json->{score},
                    best_score    => $result_json->{score},
                    latest_status => $result_json->{pass} ? 'pass' : 'fail',
                    created_at    => \'UNIX_TIMESTAMP()',
                    updated_at    => \'UNIX_TIMESTAMP()',
                },
                {
                    latest_score  => \'VALUES(latest_score)',
                    best_score    => \'GREATEST(best_score, VALUES(best_score))',
                    latest_status => \'VALUES(latest_status)',
                    updated_at    => \'VALUES(updated_at)',
                },
            );
            $dbh->do($stmt, undef, @bind);

            return unless $result_json->{pass};

            ($stmt, @bind) = $self->sql->insert(
                'all_scores',
                {
                    team_id    => $team_id,
                    score      => $result_json->{score},
                    created_at => \'UNIX_TIMESTAMP()',
                },
            );
            $dbh->do($stmt, undef, @bind);
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return;
}

sub abort_job {
    my ($self, $job_id, $result_json, $log) = @_;

    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->update(
                'bench_queues',
                {
                    state       => JOB_QUEUE_STATE_ABORTED,
                    updated_at  => \'UNIX_TIMESTAMP()',
                    result_json => $self->json->encode($result_json),
                    log_text    => $log,
                },
                {
                    id    => $job_id,
                    state => JOB_QUEUE_STATE_RUNNING,
                },
            );
            $dbh->do($stmt, undef, @bind);
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return;
}

# TODO: 定期的に叩く奴が必要
sub abort_timeout_job {
    my ($self) = @_;
    my $result_json = { reason => 'Benchmark timeout' };

    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->update(
                'bench_queues',
                {
                    state       => JOB_QUEUE_STATE_ABORTED,
                    result_json => $self->json->encode($result_json),
                    updated_at  => \'UNIX_TIMESTAMP()',
                },
                {
                    state      => JOB_QUEUE_STATE_RUNNING,
                    updated_at => { '<' => \'NOW() - INTERVAL 150 SECOND' },
                },
            );
            $dbh->do($stmt, undef, @bind);
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return;
}

1;
