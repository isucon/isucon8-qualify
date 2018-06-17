package Torb::Web;
use strict;
use warnings;
use utf8;

use Kossy;
use DBIx::Sunny;

filter login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        # TODO
        $app->($self, $c);
    };
};

filter fillin_user => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        # TODO
        $app->($self, $c);
    };
};

sub dbh {
    my $self = shift;
    $self->{_dbh} ||= do {
        my $dsn = "dbi:mysql:database=$ENV{DB_DATABASE};host=$ENV{DB_HOST};port=$ENV{DB_PORT}";
        DBIx::Sunny->connect($dsn, $ENV{DB_USER}, $ENV{DB_PASS}, {
            mysql_enable_utf8mb4 => 1,
            mysql_auto_reconnect => 1,
        });
    };
}

get '/' => [qw/fillin_user/] => sub {
    my ($self, $c) = @_;
};

post '/api/users' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

post '/api/actions/login' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

post '/api/actions/logout' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

get '/api/events' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

get '/api/events/{id}' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

post '/api/events/{id}/actions/reserve' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

post '/api/events/{id}/actions/reserve' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

router ['DELETE'] => '/api/events/{id}/sheets/{num}' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
};

filter admin_login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        # TODO
        $app->($self, $c);
    };
};

filter fillin_admin_user => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        # TODO
        $app->($self, $c);
    };
};

get '/admin/' => [qw/fillin_admin_user/] => sub {
    my ($self, $c) = @_;
};

post '/admin/api/actions/login' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

post '/admin/api/actions/logout' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

get '/admin/api/events' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

post '/admin/api/events' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

get '/admin/api/events/{id}' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

router ['PUT'] => '/admin/api/events/{id}' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

get '/admin/api/reports/events/{id}/sales' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

get '/admin/api/reports/sales' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
};

1;

