package ISUCON8::Portal::Model::Admin;

use strict;
use warnings;
use feature 'state';
use parent 'ISUCON8::Portal::Model';
use Encode;

use ISUCON8::Portal::Exception;
use ISUCON8::Portal::Constants::Common;

use Mouse;

__PACKAGE__->meta->make_immutable;

no Mouse;

sub find_user {
    my ($self, $params) = @_;
    my $name     = $params->{name};
    my $password = $params->{password};

    my $user;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'admin_users',
                ['*'],
                {
                    name     => $name,
                    password => $password, # TODO: password hash
                },
            );
            $user = $dbh->selectrow_hashref($stmt, undef, @bind);
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

    return $user;
}

sub get_all_jobs {
    my ($self, $params) = @_;

    my $jobs = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                { 'bench_queues' => 'b' },
                [
                    qw/b.id b.team_id b.state b.result_score b.result_status b.updated_at/,
                    { 't.name' => 'team_name' },
                ],
                {},
                {
                    order_by => { -desc => 'id' },
                    join     => {
                        table     => { teams => 't' },
                        condition => { 'b.team_id' => 't.id' },
                    },
                    limit => 1000,
                },
            );
            $jobs = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
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

    return $jobs;
}

sub get_processing_jobs {
    my ($self, $params) = @_;

    my $jobs = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                { 'bench_queues' => 'b' },
                [
                    \'b.*',
                    { 't.name' => 'team_name' },
                ],
                {
                    'b.state' => [ JOB_QUEUE_STATE_WAITING, JOB_QUEUE_STATE_RUNNING ],
                },
                {
                    order_by => { -asc => 'id' },
                    join     => {
                        table     => { teams => 't' },
                        condition => { 'b.team_id' => 't.id' },
                    },
                },
            );
            $jobs = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
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

    return $jobs;
}

sub get_job {
    my ($self, $params) = @_;
    my $job_id= $params->{job_id};

    my $job;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                ['*'],
                {
                    id => $job_id,
                },
            );
            $job = $dbh->selectrow_hashref($stmt, undef, @bind);
            return unless $job;

            ($stmt, @bind) = $self->sql->select(
                'teams',
                ['*'],
                {
                    id => $job->{team_id},
                },
            );
            $job->{team} = $dbh->selectrow_hashref($stmt, undef, @bind);

            $job->{result_json} = $self->json->decode(encode_utf8 $job->{result_json} || '{}');
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

sub update_information {
    my ($self, $params) = @_;
    my $message = $params->{message};

    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->update(
                'informations',
                {
                    message    => $message,
                    updated_at => \'UNIX_TIMESTAMP()',
                },
            );
            my $rv = $dbh->do($stmt, undef, @bind);

            if ($rv == 0) {
                ($stmt, @bind) = $self->sql->insert(
                    'informations',
                    {
                        message    => $message,
                        updated_at => \'UNIX_TIMESTAMP()',
                    },
                );
                $dbh->do($stmt, undef, @bind);
            }
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

sub get_servers {
    my ($self, $params) = @_;

    my $servers = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                { teams => 't' },
                [
                    \'s.*',
                    { 't.id'    => 'team_id' },
                    { 't.name'  => 'team_name' },
                    { 't.state' => 'team_state' },
                ],
                {},
                {
                    order_by => [
                        { -asc => 't.id' },
                        { -asc => 's.private_ip' },
                    ],
                    join     => {
                        table     => { servers => 's' },
                        condition => { 't.group_id' => 's.group_id' },
                    },
                },
            );
            $servers = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
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

    return $servers;
}

sub update_team {
    my ($self, $params) = @_;
    my $team_id = $params->{id};
    my $message = $params->{message};
    my $state   = $params->{state};
    my $note    = $params->{note};

    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->update(
                'teams',
                {
                    state      => $state,
                    message    => $message,
                    note       => $note,
                    updated_at => \'UNIX_TIMESTAMP()',
                },
                {
                    id => $team_id,
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
