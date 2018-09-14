package Torb::Web;
use strict;
use warnings;
use utf8;

use Kossy;

use JSON::XS 3.00;
use DBIx::Sunny;
use Plack::Session;
use Time::Moment;
use File::Spec;

filter login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;

        my $user = $self->get_login_user($c);
        return $self->res_error($c, login_required => 401) unless $user;

        $app->($self, $c);
    };
};

filter fillin_user => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;

        my $user = $self->get_login_user($c);
        $c->stash->{user} = $user if $user;

        $app->($self, $c);
    };
};

filter allow_json_request => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        $c->env->{'kossy.request.parse_json_body'} = 1;
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
            # TODO: replace mysqld's sql_mode setting and remove following codes
            Callbacks => {
                connected => sub {
                    my $dbh = shift;
                    $dbh->do('SET SESSION sql_mode="STRICT_TRANS_TABLES,NO_ZERO_IN_DATE,NO_ZERO_DATE,ERROR_FOR_DIVISION_BY_ZERO,NO_ENGINE_SUBSTITUTION"');
                    return;
                },
            },
        });
    };
}

get '/' => [qw/fillin_user/] => sub {
    my ($self, $c) = @_;

    my @events = map { $self->sanitize_event($_) } $self->get_events();
    return $c->render('index.tx', {
        events      => \@events,
        encode_json => sub { $c->escape_json(JSON::XS->new->encode(@_)) },
    });
};

get '/initialize' => sub {
    my ($self, $c) = @_;

    system+File::Spec->catfile($self->root_dir, '../../db/init.sh');

    return $c->req->new_response(204, [], '');
};

post '/api/users' => [qw/allow_json_request/] => sub {
    my ($self, $c) = @_;
    my $nickname   = $c->req->body_parameters->get('nickname');
    my $login_name = $c->req->body_parameters->get('login_name');
    my $password   = $c->req->body_parameters->get('password');

    my ($user_id, $error);

    my $res;
    my $txn = $self->dbh->txn_scope();
    eval {
        my $duplicated = $self->dbh->select_one('SELECT * FROM users WHERE login_name = ?', $login_name);
        if ($duplicated) {
            $res = $self->res_error($c, duplicated => 409);
            $txn->rollback();
            return;
        }

        $self->dbh->query('INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)', $login_name, $password, $nickname);
        $user_id = $self->dbh->last_insert_id();
        $txn->commit();
    };
    if ($@) {
        warn "rollback by: $@";
        $txn->rollback();
        $res = $self->res_error($c);
    }
    return $res if $res;

    $res = $c->render_json({ id => 0+$user_id, nickname => $nickname });
    $res->status(201);
    return $res;
};

sub get_login_user {
    my ($self, $c) = @_;

    my $session = Plack::Session->new($c->env);
    my $user_id = $session->get('user_id');
    return unless $user_id;
    return $self->dbh->select_row('SELECT id, nickname FROM users WHERE id = ?', $user_id);
}

get '/api/users/{id}' => [qw/login_required/] => sub {
    my ($self, $c) = @_;

    my $user = $self->dbh->select_row('SELECT id, nickname FROM users WHERE id = ?', $c->args->{id});
    if ($user->{id} != $self->get_login_user($c)->{id}) {
        return $self->res_error($c, forbidden => 403);
    }

    my @recent_reservations;
    {
        my $rows = $self->dbh->select_all('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5', $user->{id});
        for my $row (@$rows) {
            my $event = $self->get_event($row->{event_id});

            my $reservation = {
                id          => 0+$row->{id},
                event       => $event,
                sheet_rank  => $row->{sheet_rank},
                sheet_num   => 0+$row->{sheet_num},
                price       => $event->{sheets}->{$row->{sheet_rank}}->{price},
                reserved_at => Time::Moment->from_string("$row->{reserved_at}Z", lenient => 1)->epoch(),
                canceled_at => $row->{canceled_at} ? Time::Moment->from_string("$row->{canceled_at}Z", lenient => 1)->epoch() : undef,
            };
            push @recent_reservations => $reservation;

            delete $event->{sheets};
            delete $event->{total};
            delete $event->{remains};
            delete $event->{price};
        }
    };
    $user->{recent_reservations} = \@recent_reservations;
    $user->{total_price} = 0+$self->dbh->select_one('SELECT IFNULL(SUM(e.price + s.price), 0) FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL', $user->{id});

    my @recent_events;
    {
        my $rows = $self->dbh->select_all('SELECT event_id FROM reservations WHERE user_id = ? GROUP BY event_id ORDER BY MAX(IFNULL(canceled_at, reserved_at)) DESC LIMIT 5', $user->{id});
        for my $row (@$rows) {
            my $event = $self->get_event($row->{event_id});
            delete $event->{sheets}->{$_}->{detail} for keys %{ $event->{sheets} };

            push @recent_events => $event;
        }
    }
    $user->{recent_events} = \@recent_events;

    return $c->render_json($user);
};

post '/api/actions/login' => [qw/allow_json_request/] => sub {
    my ($self, $c) = @_;
    my $login_name = $c->req->body_parameters->get('login_name');
    my $password   = $c->req->body_parameters->get('password');

    my $user      = $self->dbh->select_row('SELECT * FROM users WHERE login_name = ?', $login_name);
    my $pass_hash = $self->dbh->select_one('SELECT SHA2(?, 256)', $password);
    return $self->res_error($c, authentication_failed => 401) if !$user || $pass_hash ne $user->{pass_hash};

    my $session = Plack::Session->new($c->env);
    $session->set('user_id' => $user->{id});

    $user = $self->get_login_user($c);
    return $c->render_json($user);
};

post '/api/actions/logout' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
    my $session = Plack::Session->new($c->env);
    $session->remove('user_id');
    return $c->req->new_response(204, [], '');
};

get '/api/events' => sub {
    my ($self, $c) = @_;
    my @events = map { $self->sanitize_event($_) } $self->get_events();
    return $c->render_json(\@events);
};

get '/api/events/{id}' => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};

    my $user = $self->get_login_user($c) || {};
    my $event = $self->get_event($event_id, $user->{id});
    return $self->res_error($c, not_found => 404) if !$event || !$event->{public};

    $event = $self->sanitize_event($event);
    return $c->render_json($event);
};

sub get_events {
    my ($self, $where) = @_;
    $where ||= sub { $_->{public_fg} };

    my $txn = $self->dbh->txn_scope();

    my @events;
    my @event_ids = map { $_->{id} } grep $where->($_), @{ $self->dbh->select_all('SELECT * FROM events ORDER BY id ASC') };
    for my $event_id (@event_ids) {
        my $event = $self->get_event($event_id);

        delete $event->{sheets}->{$_}->{detail} for keys %{ $event->{sheets} };
        push @events => $event;
    }

    $txn->commit();

    return @events;
}

sub get_event {
    my ($self, $event_id, $login_user_id) = @_;

    my $event = $self->dbh->select_row('SELECT * FROM events WHERE id = ?', $event_id);
    return unless $event;

    # zero fill
    $event->{total}   = 0;
    $event->{remains} = 0;
    for my $rank (qw/S A B C/) {
        $event->{sheets}->{$rank}->{total}   = 0;
        $event->{sheets}->{$rank}->{remains} = 0;
    }

    my $sheets = $self->dbh->select_all('SELECT * FROM sheets ORDER BY `rank`, num');
    for my $sheet (@$sheets) {
        $event->{sheets}->{$sheet->{rank}}->{price} ||= $event->{price} + $sheet->{price};

        $event->{total} += 1;
        $event->{sheets}->{$sheet->{rank}}->{total} += 1;

        my $reservation = $self->dbh->select_row('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MIN(reserved_at)', $event->{id}, $sheet->{id});
        if ($reservation) {
            $sheet->{mine}        = JSON::XS::true if $login_user_id && $reservation->{user_id} == $login_user_id;
            $sheet->{reserved}    = JSON::XS::true;
            $sheet->{reserved_at} = Time::Moment->from_string($reservation->{reserved_at}.'Z', lenient => 1)->epoch;
        } else {
            $event->{remains} += 1;
            $event->{sheets}->{$sheet->{rank}}->{remains} += 1;
        }

        push @{ $event->{sheets}->{$sheet->{rank}}->{detail} } => $sheet;

        delete $sheet->{id};
        delete $sheet->{price};
        delete $sheet->{rank};
    }

    $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
    $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;

    return $event;
}

sub sanitize_event {
    my ($self, $event) = @_;
    my $sanitized = {%$event}; # shallow clone
    delete $sanitized->{price};
    delete $sanitized->{public};
    delete $sanitized->{closed};
    return $sanitized;
}

post '/api/events/{id}/actions/reserve' => [qw/allow_json_request login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $rank = $c->req->body_parameters->get('sheet_rank');

    my $user  = $self->get_login_user($c);
    my $event = $self->get_event($event_id, $user->{id});
    return $self->res_error($c, invalid_event => 404) unless $event && $event->{public};
    return $self->res_error($c, invalid_rank => 400)  unless $self->validate_rank($rank);

    my $sheet;
    my $reservation_id;
    while (1) {
        $sheet = $self->dbh->select_row('SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1', $event->{id}, $rank);
        return $self->res_error($c, sold_out => 409) unless $sheet;

        my $txn = $self->dbh->txn_scope();
        eval {
            $self->dbh->query('INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)', $event->{id}, $sheet->{id}, $user->{id}, Time::Moment->now_utc->strftime('%F %T%f'));
            $reservation_id = $self->dbh->last_insert_id();
            $txn->commit();
        };
        if ($@) {
            $txn->rollback();
            warn "re-try: rollback by $@";
            next; # retry
        }

        last;
    }

    my $res = $c->render_json({
        id         => 0+$reservation_id,
        sheet_rank => $rank,
        sheet_num  => 0+$sheet->{num},
    });
    $res->status(202);
    return $res;
};

router ['DELETE'] => '/api/events/{id}/sheets/{rank}/{num}/reservation' => [qw/login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $rank     = $c->args->{rank};
    my $num      = $c->args->{num};

    my $user  = $self->get_login_user($c);
    my $event = $self->get_event($event_id, $user->{id});
    return $self->res_error($c, invalid_event => 404) unless $event && $event->{public};
    return $self->res_error($c, invalid_rank => 404)  unless $self->validate_rank($rank);

    my $sheet = $self->dbh->select_row('SELECT * FROM sheets WHERE `rank` = ? AND num = ?', $rank, $num);
    return $self->res_error($c, invalid_sheet => 404)  unless $sheet;

    my $res;
    my $txn = $self->dbh->txn_scope();
    eval {
        my $reservation = $self->dbh->select_row('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MIN(reserved_at) FOR UPDATE', $event->{id}, $sheet->{id});
        unless ($reservation) {
            $res = $self->res_error($c, not_reserved => 400);
            $txn->rollback();
            return;
        }
        if ($reservation->{user_id} != $user->{id}) {
            $res = $self->res_error($c, not_permitted => 403);
            $txn->rollback();
            return;
        }

        $self->dbh->query('UPDATE reservations SET canceled_at = ? WHERE id = ?', Time::Moment->now_utc->strftime('%F %T%f'), $reservation->{id});
        $txn->commit();
    };
    if ($@) {
        warn "rollback by: $@";
        $txn->rollback();
        $res = $self->res_error($c);
    }
    return $res if $res;

    return $c->req->new_response(204, [], '');
};

sub validate_rank {
    my ($self, $rank) = @_;
    return $self->dbh->select_one('SELECT COUNT(*) FROM sheets WHERE `rank` = ?', $rank);
}

filter admin_login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        my $session = Plack::Session->new($c->env);

        my $administrator = $self->get_login_administrator($c);
        return $self->res_error($c, admin_login_required => 401) unless $administrator;

        $app->($self, $c);
    };
};

filter fillin_administrator => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;

        my $administrator = $self->get_login_administrator($c);
        $c->stash->{administrator} = $administrator if $administrator;

        $app->($self, $c);
    };
};

get '/admin/' => [qw/fillin_administrator/] => sub {
    my ($self, $c) = @_;

    my @events;
    @events = $self->get_events(sub { $_ }) if $c->stash->{administrator};

    return $c->render('admin.tx', {
        events      => \@events,
        encode_json => sub { $c->escape_json(JSON::XS->new->encode(@_)) },
    });
};

post '/admin/api/actions/login' => [qw/allow_json_request/] => sub {
    my ($self, $c) = @_;
    my $login_name = $c->req->body_parameters->get('login_name');
    my $password   = $c->req->body_parameters->get('password');

    my $administrator = $self->dbh->select_row('SELECT * FROM administrators WHERE login_name = ?', $login_name);
    my $pass_hash     = $self->dbh->select_one('SELECT SHA2(?, 256)', $password);
    return $self->res_error($c, authentication_failed => 401) if !$administrator || $pass_hash ne $administrator->{pass_hash};

    my $session = Plack::Session->new($c->env);
    $session->set('administrator_id' => $administrator->{id});

    $administrator = $self->get_login_administrator($c);
    return $c->render_json($administrator);
};

post '/admin/api/actions/logout' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $session = Plack::Session->new($c->env);
    $session->remove('administrator_id');
    return $c->req->new_response(204, [], '');
};

sub get_login_administrator {
    my ($self, $c) = @_;
    my $session = Plack::Session->new($c->env);
    my $administrator_id = $session->get('administrator_id');
    return unless $administrator_id;
    return $self->dbh->select_row('SELECT id, nickname FROM administrators WHERE id = ?', $administrator_id);
}

get '/admin/api/events' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;

    my @events = $self->get_events(sub { $_ });
    return $c->render_json(\@events);
};

post '/admin/api/events' => [qw/allow_json_request admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $title  = $c->req->body_parameters->get('title');
    my $public = $c->req->body_parameters->get('public') ? 1 : 0;
    my $price  = $c->req->body_parameters->get('price');

    my $event_id;

    my $txn = $self->dbh->txn_scope();
    eval {
        $self->dbh->query('INSERT INTO events (title, public_fg, closed_fg, price) VALUES (?, ?, 0, ?)', $title, $public, $price);
        $event_id = $self->dbh->last_insert_id();
        $txn->commit();
    };
    if ($@) {
        $txn->rollback();
    }

    my $event = $self->get_event($event_id);
    return $c->render_json($event);
};

get '/admin/api/events/{id}' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};

    my $event = $self->get_event($event_id);
    return $self->res_error($c, not_found => 404) unless $event;

    return $c->render_json($event);
};

post '/admin/api/events/{id}/actions/edit' => [qw/allow_json_request admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $public = $c->req->body_parameters->get('public') ? 1 : 0;
    my $closed = $c->req->body_parameters->get('closed') ? 1 : 0;
    $public = 0 if $closed;

    my $event = $self->get_event($event_id);
    return $self->res_error($c, not_found => 404) unless $event;

    if ($event->{closed}) {
        return $self->res_error($c, cannot_edit_closed_event => 400);
    } elsif ($event->{public} && $closed) {
        return $self->res_error($c, cannot_close_public_event => 400);
    }

    my $txn = $self->dbh->txn_scope();
    eval {
        $self->dbh->query('UPDATE events SET public_fg = ?, closed_fg = ? WHERE id = ?', $public, $closed, $event->{id});
        $txn->commit();
    };
    if ($@) {
        $txn->rollback();
    }

    $event = $self->get_event($event_id);
    return $c->render_json($event);
};

get '/admin/api/reports/events/{id}/sales' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $event = $self->get_event($event_id);

    my @reports;

    my $reservations = $self->dbh->select_all('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.event_id = ? ORDER BY reserved_at ASC FOR UPDATE', $event->{id});
    for my $reservation (@$reservations) {
        my $report = {
            reservation_id => $reservation->{id},
            event_id       => $event->{id},
            rank           => $reservation->{sheet_rank},
            num            => $reservation->{sheet_num},
            user_id        => $reservation->{user_id},
            sold_at        => Time::Moment->from_string("$reservation->{reserved_at}Z", lenient => 1)->to_string(),
            canceled_at    => $reservation->{canceled_at} ? Time::Moment->from_string("$reservation->{canceled_at}Z", lenient => 1)->to_string() : '',
            price          => $reservation->{event_price} + $reservation->{sheet_price},
        };

        push @reports => $report;
    }

    return $self->render_report_csv($c, \@reports);
};

get '/admin/api/reports/sales' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;

    my @reports;

    my $reservations = $self->dbh->select_all('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num, s.price AS sheet_price, e.id AS event_id, e.price AS event_price FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id ORDER BY reserved_at ASC FOR UPDATE');
    for my $reservation (@$reservations) {
        my $report = {
            reservation_id => $reservation->{id},
            event_id       => $reservation->{event_id},
            rank           => $reservation->{sheet_rank},
            num            => $reservation->{sheet_num},
            user_id        => $reservation->{user_id},
            sold_at        => Time::Moment->from_string("$reservation->{reserved_at}Z", lenient => 1)->to_string(),
            canceled_at    => $reservation->{canceled_at} ? Time::Moment->from_string("$reservation->{canceled_at}Z", lenient => 1)->to_string() : '',
            price          => $reservation->{event_price} + $reservation->{sheet_price},
        };

        push @reports => $report;
    }

    return $self->render_report_csv($c, \@reports);
};

sub render_report_csv {
    my ($self, $c, $reports) = @_;
    my @reports = sort { $a->{sold_at} cmp $b->{sold_at} } @$reports;

    my @keys = qw/reservation_id event_id rank num price user_id sold_at canceled_at/;
    my $body = join ',', @keys;
    $body .= "\n";
    for my $report (@reports) {
        $body .= join ',', @{$report}{@keys};
        $body .= "\n";
    }

    my $res = $c->req->new_response(200, [
        'Content-Type'        => 'text/csv; charset=UTF-8',
        'Content-Disposition' => 'attachment; filename="report.csv"',
    ], $body);
    return $res;
}

sub res_error {
    my ($self, $c, $error, $status) = @_;
    $error  ||= 'unknown';
    $status ||= 500;

    my $res = $c->render_json({ error => $error });
    $res->status($status);
    return $res;
}

1;
