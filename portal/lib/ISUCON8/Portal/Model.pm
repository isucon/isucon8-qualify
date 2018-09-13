package ISUCON8::Portal::Model;

use strict;
use warnings;
use feature 'state';

use Time::Piece;
use Tie::IxHash;
use URI;
use Encode;
use Digest::SHA qw(sha1_hex);
use Digest::MurmurHash3 qw(murmur128_x64);
use MIME::Base64 qw(encode_base64url decode_base64url);
use File::Slurp qw(read_file write_file);
use Data::Recursive::Encode;
use Capture::Tiny qw(capture);
use Furl;
use IO::Socket::SSL qw/SSL_VERIFY_NONE/;

use Crypt::CBC;
use Crypt::OpenSSL::AES;

use ISUCON8::Portal::Exception;
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

1;
