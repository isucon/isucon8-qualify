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
                {
                    limit => 1,
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
