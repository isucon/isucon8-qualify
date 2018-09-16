package ISUCON8::Portal::Model::Team;

use strict;
use warnings;
use feature 'state';
use parent 'ISUCON8::Portal::Model';

use ISUCON8::Portal::Exception;
use ISUCON8::Portal::Constants::Common;
use Encode qw(encode_utf8);
use Time::Piece;
use List::Util qw(uniq);

use Mouse;

__PACKAGE__->meta->make_immutable;

no Mouse;

sub find_team {
    my ($self, $params) = @_;
    my $id       = $params->{id};
    my $password = $params->{password};

    my $team;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'teams',
                ['*'],
                {
                    id       => $id,
                    password => $password, # TODO: password hash
                },
            );
            $team = $dbh->selectrow_hashref($stmt, undef, @bind);
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

    return $team;
}

sub get_team {
    my ($self, $params) = @_;
    my $id = $params->{id};

    my $team;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'teams',
                ['*'],
                {
                    id => $id,
                },
            );
            $team = $dbh->selectrow_hashref($stmt, undef, @bind);
            return unless $team;

            ($stmt, @bind) = $self->sql->select(
                'team_members',
                ['*'],
                {
                    team_id => $id,
                },
                {
                    order_by => { -asc => 'member_number' },
                },
            );
            $team->{members} = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);

            $team->{category_display_name} =
                TEAM_CATEGORY_TO_DISPLAY_NAME_MAP->{ $team->{category} };
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

    return $team;
}

sub get_servers {
    my ($self, $params) = @_;
    my $group_id = $params->{group_id};

    my $servers = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'servers',
                ['*'],
                {
                    group_id => $group_id,
                },
                {
                    order_by => { -asc => 'private_ip' },
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

sub get_latest_score {
    my ($self, $params) = @_;
    my $team_id = $params->{team_id};

    my $score;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'team_scores',
                ['*'],
                {
                    team_id => $team_id,
                },
            );
            $score = $dbh->selectrow_hashref($stmt, undef, @bind);
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

    return $score;
}

sub get_team_job {
    my ($self, $params) = @_;
    my $team_id = $params->{team_id};
    my $job_id  = $params->{job_id};

    my $job;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                ['*'],
                {
                    id      => $job_id,
                    team_id => $team_id,
                },
            );
            $job = $dbh->selectrow_hashref($stmt, undef, @bind);
            return unless $job;

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

sub get_team_jobs {
    my ($self, $params) = @_;
    my $team_id = $params->{team_id};
    my $limit   = $params->{limit};

    my $jobs = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'bench_queues',
                [qw/id team_id state result_status result_score updated_at/],
                {
                    team_id => $team_id,
                },
                {
                    order_by => { -desc => 'updated_at' },
                    $limit ? (limit => $limit) : (),
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

sub change_benchmark_target {
    my ($self, $params) = @_;
    my $group_id  = $params->{group_id};
    my $global_ip = $params->{global_ip};

    my $is_success = 0;
    my $err        = undef;
    eval {
        $self->db->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->update(
                'servers',
                {
                    is_target_host => \['IF(global_ip = ?, 1, 0)', $global_ip],
                    updated_at     => \'UNIX_TIMESTAMP()',
                },
                {
                    group_id => $group_id,
                },
            );
            my $rc = $dbh->do($stmt, undef, @bind);
            unless ($rc > 0) {
                $err = 'Affected Rows = 0. Really?';
                return;
            }
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

1;
