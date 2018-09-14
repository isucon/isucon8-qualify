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

            $team->{category_display_name} = TEAM_CATEGORY_TO_DISPLAY_NAME_MAP->{ $team->{category} };
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

sub get_teams {
    my ($self, $params) = @_;
    my $ids = $params->{ids};

    my $teams = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'teams',
                ['*'],
                {
                    @$ids ? (id => $ids) : (),
                },
            );
            $teams = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
            for my $row (@$teams) {
                $row->{category_display_name} = TEAM_CATEGORY_TO_DISPLAY_NAME_MAP->{ $row->{category} };
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

    return $teams;
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
                    order_by => { -asc => 'global_ip' },
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

sub get_team_scores {
    my ($self, $params) = @_;
    my $limit = $params->{limit};

    my $scores = [];
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                { teams => 't' },
                [
                    's.team_id', 's.latest_score', 's.best_score', 's.updated_at',
                    's.latest_status', 't.name', 't.category',
                ],
                {
                    't.state'      => TEAM_STATE_ACTIVED,
                    's.best_score' => \'IS NOT NULL',
                },
                {
                    join => {
                        type      => 'LEFT',
                        table     => { team_scores => 's' },
                        condition => { 't.id' => 's.team_id' },
                    },
                    order_by => [
                        { -desc => 's.latest_score' },
                        { -asc  => 't.id' },
                    ],
                    $limit ? (limit => $limit) : (),
                },
            );
            $scores = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);

            for my $row (@$scores) {
                $row->{category_display_name} = TEAM_CATEGORY_TO_DISPLAY_NAME_MAP->{ $row->{category} };
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

    return $scores;
}

sub get_chart_data {
    my ($self, $params) = @_;
    my $team_id         = $params->{team_id};
    my $is_last_spurt   = $params->{is_last_spurt};
    my $last_spurt_time = $params->{last_spurt_time};
    my $limit           = $params->{limit};

    my $char_data = {};
    eval {
        my $scores   = [];
        my $team_ids = [];
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                { teams => 't' },
                ['team_id'],
                {
                    't.state' => TEAM_STATE_ACTIVED,
                    $is_last_spurt ? (
                        's.created_at' => { '<' => $last_spurt_time },
                    ) : (),
                },
                {
                    join => {
                        table     => { all_scores => 's' },
                        condition => { 't.id' => 's.team_id' },
                    },
                    group_by => 's.team_id',
                    order_by => { -desc => \'MAX(s.score)' },
                    $limit ? (limit => $limit) : (),
                },
            );
            $team_ids = $dbh->selectcol_arrayref($stmt, undef, @bind);

            ($stmt, @bind) = $self->sql->select(
                'all_scores',
                ['*'],
                {
                    team_id => $team_ids,
                },
                {
                    order_by => { -asc => 'created_at' },
                },
            );
            $scores = $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
        });
        return unless @$scores;

        my $min_time = do {
            my $t   = localtime($scores->[0]{created_at});
            my $min = $t->min < 30 ? 0 : 30;
            my $datetime = sprintf(
                '%04d-%02d-%02d %02d:%02d:00',
                $t->year, $t->mon, $t->mday, $t->hour, $min,
            );
            $self->unixtime_stamp($datetime);
        };
        my $max_time = do {
            my $t = localtime();
            my $min;
            if ($t->min < 30) {
                $min = 30;
            }
            else {
                $t = $t + 60 * 60;
                $min = 0;
            }
            my $datetime = sprintf(
                '%04d-%02d-%02d %02d:%02d:00',
                $t->year, $t->mon, $t->mday, $t->hour, $min,
            );
            $self->unixtime_stamp($datetime);
        };

        my $labels = [ $min_time, $max_time ];
        my $team_score_map = {};
        for my $row (@$scores) {
            push @$labels, $row->{created_at};
            push @{ $team_score_map->{ $row->{team_id} } }, $row;
        }
        $labels = [ uniq sort { $a <=> $b } @$labels ];

        $char_data->{labels} = $labels;

        my $teams    = $self->get_teams({ ids => $team_ids });
        my $team_map = { map { $_->{id} => $_ } @$teams };
        my $list     = [];
        for my $id (@$team_ids) {
            my $team   = $team_map->{ $id };
            my $scores = $team_score_map->{ $id };
            my $data   = [];
            for my $label (@$labels) {
                if (scalar @$scores) {
                    my $created_at = $scores->[0]{created_at};
                    if ($is_last_spurt && $id != $team_id && $created_at > $last_spurt_time) {
                        push @$data, undef;
                    }
                    else {
                        if ($label == $created_at) {
                            push @$data, shift(@$scores)->{score};
                        }
                        else {
                            push @$data, $data->[-1];
                        }
                    }
                }
                else {
                    push @$data, undef;
                }
            }

            push @$list, {
                team   => $team,
                scores => $data,
            };
        }

        $char_data->{list}   = $list;
        $char_data->{labels} = [
            map { $self->from_unixtime($_) } @{ $char_data->{labels} }
        ];
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return $char_data;
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
