package ISUCON8::Portal::Model;

use strict;
use warnings;
use feature 'state';

use Time::Piece;
use Tie::IxHash;
use URI;
use Encode;
use Digest::MurmurHash3 qw(murmur128_x64);
use MIME::Base64 qw(encode_base64url decode_base64url);
use Data::Recursive::Encode;
use List::Util qw(uniq);

use ISUCON8::Portal::Exception;
use ISUCON8::Portal::Constants::Common;
use ISUCON8::Portal::Web::ViewFunctions();

use Mouse;

has container => (
    is  => 'ro',
    isa => 'ISUCON8::Portal::Container',
);

__PACKAGE__->meta->make_immutable;

no Mouse;

sub log {
    state $log = shift->container->get('Log');
}

sub db {
    state $db = shift->container->get('DB');
}

sub sql {
    state $sql = shift->container->get('SQL');
}

sub json {
    state $json = shift->container->get('JSON');
}

sub config {
    shift->container->config;
}

sub model {
    my $self = shift;
    $self->container->get('ISUCON8::Portal::Model::'.$_[0]);
}

sub ordered_hash {
    my ($self, @hash) = @_;
    tie my %h, 'Tie::IxHash', @hash;
    return \%h;
}

sub unixtime_stamp {
    my ($self, $datetime) = @_;
    localtime(Time::Piece->strptime($datetime, '%Y-%m-%d %H:%M:%S'))->epoch;
}

sub from_unixtime {
    my ($self, $unixtime) = @_;
    localtime($unixtime)->strftime("%Y-%m-%d %H:%M:%S")
}

sub recursive_decode {
    my ($self, $data) = @_;
    Data::Recursive::Encode->decode_utf8($data);
}

sub recursive_encode {
    my ($self, $data) = @_;
    Data::Recursive::Encode->encode_utf8($data);
}

sub get_information {
    my ($self, $params) = @_;

    my $message;
    eval {
        $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'informations',
                ['*'],
            );
            $message = $dbh->selectrow_hashref($stmt, undef, @bind);
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

    return $message;
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
                    't.state'      => TEAM_STATE_ACTIVE,
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

sub get_teams {
    my ($self, $params) = @_;
    my $ids = $params->{ids} || [];

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

sub get_chart_data {
    my ($self, $params) = @_;
    my $team_id         = $params->{team_id};
    my $is_last_spurt   = $params->{is_last_spurt};
    my $last_spurt_time = $params->{last_spurt_time};
    my $limit           = $params->{limit};

    my $chart_data = {};
    eval {
        my $scores   = [];
        my $team_ids = [];
        $self->db->run(sub {
            my $dbh = shift;
            my $stmt = << '__SQL__';
SELECT A.team_id, A.score, FROM_UNIXTIME(A.created_at) FROM all_scores A
INNER JOIN (SELECT team_id, MAX(created_at) max_created_at FROM all_scores GROUP BY team_id) B
ON A.team_id = B.team_id AND A.created_at = B.max_created_at
WHERE A.created_at <= ? ORDER BY A.score DESC LIMIT ?
__SQL__
            $team_ids = $dbh->selectcol_arrayref(
                $stmt,
                undef,
                $is_last_spurt ? $last_spurt_time : time, $limit,
            );
            if ($team_id) {
                $team_ids = [ uniq(@$team_ids, $team_id) ];
            }

            ($stmt, my @bind) = $self->sql->select(
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

        $chart_data->{labels} = $labels;

        my $teams    = $self->get_teams({ ids => $team_ids });
        my $team_map = { map { $_->{id} => $_ } @$teams };
        my $list     = [];
        for my $id (@$team_ids) {
            my $team   = $team_map->{ $id };
            my $scores = $team_score_map->{ $id } || next;
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
                            push @$data, undef;
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

        $chart_data->{list}   = $list;
        $chart_data->{labels} = [
            map { $self->from_unixtime($_) } @{ $chart_data->{labels} }
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

    return $chart_data;
}

sub get_team_chart_data {
    my ($self, $params) = @_;
    my $team_id = $params->{team_id};

    my $chart_data = {};
    eval {
        my $scores = $self->db->run(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->select(
                'all_scores',
                ['*'],
                {
                    team_id => $team_id,
                },
                {
                    -asc => 'created_at',
                },
            );
            $dbh->selectall_arrayref($stmt, { Slice => {} }, @bind);
        });
        my ($team) = @{ $self->get_teams({ ids => [ $team_id ] }) };
        my $labels = [];
        my $data   = [];
        for my $row (@$scores) {
            push @$labels, $row->{created_at};
            push @$data, $row->{score};
        }
        $chart_data->{list}   = [ { team => $team, scores => $data } ];
        $chart_data->{labels} = [ map { $self->from_unixtime($_) } @$labels ];
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'ISUCON8::Portal::Exception';
        ISUCON8::Portal::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return $chart_data;
}

1;
