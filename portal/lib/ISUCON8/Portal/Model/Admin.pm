package ISUCON8::Portal::Model::Admin;

use strict;
use warnings;
use feature 'state';
use parent 'ISUCON8::Portal::Model';

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
                    password => $password,
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

1;
__END__

sub create_account {
    my ($self, $params) = @_;
    my $account_name  = $params->{account_name};
    my $user_group_id = $params->{user_group_id};
    my $company       = $params->{company};
    my $daily_budget  = $params->{daily_budget};

    my $account_id;
    eval {
        $account_id = $self->db->seq('SEQ_W')->nextval('ads_accounts');
        $self->db->connect('ADS_W')->txn(sub {
            my $dbh = shift;
            my ($stmt, @bind) = $self->sql->insert(
                'ads_accounts',
                {
                    id              => $account_id,
                    user_group_id   => $user_group_id,
                    account_name    => encode_utf8($account_name),
                    company         => encode_utf8($company),
                    daily_budget    => $daily_budget,
                    publish_status  => ADS_PUBLISH_STATUS_PAUSED,
                    created_at      => \'UNIX_TIMESTAMP()',
                    updated_at      => \'UNIX_TIMESTAMP()',
                },
            );
            $dbh->do($stmt, undef, @bind);
        });
    };
    if (my $e = $@) {
        $e->rethrow if ref $e eq 'MoonShot::AdsOPE::Exception';
        MoonShot::AdsOPE::Exception->throw(
            code    => ERROR_INTERNAL_ERROR,
            message => "$e",
            logger  => sub { $self->log->critf(@_) },
        );
    }

    return $account_id;
}

1;
