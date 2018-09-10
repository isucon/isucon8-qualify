package ISUCON8::Portal::SessionStore;

use strict;
use warnings;
use Digest::MurmurHash3 qw(murmur128_x64);

sub new {
    my ($class, %args) = @_;
    bless {
        context => $args{context},
    }, $class;
}

sub c {
    shift->{context};
}

sub key_to_session_id {
    my ($self, $key) = @_;
    my ($session_id) = murmur128_x64 $key;
    return $session_id;
}

sub get {
    my ($self, $key) = @_;
    my $row = $self->c->db->run(sub {
        my $dbh = shift;
        my ($stmt, @bind) = $self->c->sql->select(
            'sessions',
            ['*'],
            {
                session_id => $self->key_to_session_id($key),
            },
        );
        $dbh->selectrow_hashref($stmt, undef, @bind);
    });
    return unless $row;
    return $self->c->json->decode($row->{session_data});
}

sub set {
    my ($self, $key, $value) = @_;
    $self->c->db->txn(sub {
        my $dbh = shift;
        my ($stmt, @bind) = $self->c->sql->insert_on_duplicate(
            'sessions',
            {
                session_id   => $self->key_to_session_id($key),
                session_data => $self->c->json->encode($value),
                created_at   => \'UNIX_TIMESTAMP()',
                updated_at   => \'UNIX_TIMESTAMP()',
            },
            {
                session_data => \'VALUES(session_data)',
                updated_at   => \'VALUES(updated_at)',
            },
        );
        $dbh->do($stmt, undef, @bind);
    });
}

sub remove {
    my ($self, $key) = @_;
    $self->c->db->txn(sub {
        my $dbh = shift;
        my ($stmt, @bind) = $self->c->sql->delete(
            'sessions',
            {
                session_id => $self->key_to_session_id($key),
            },
        );
        $dbh->do($stmt, undef, @bind);
    });
}

1;
