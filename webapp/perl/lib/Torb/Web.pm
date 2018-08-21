package Torb::Web;
use strict;
use warnings;
use utf8;

use Kossy;

use JSON::XS 3.00;
use DBIx::Sunny;
use Plack::Session;
use Time::Moment;

filter login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;

        my $user = $self->get_login_user($c);
        unless ($user) {
            my $res = $c->render_json({
                error => 'login_required',
            });
            $res->status(401);
            return $res;
        }

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

    my $txn = $self->dbh->txn_scope();
    $self->dbh->query('DELETE FROM users WHERE id > 1000');
    $self->dbh->query('DELETE FROM reservations WHERE id > 1000');
    $self->dbh->query('DELETE FROM events WHERE id > 4');
    $self->dbh->query('UPDATE events SET public_fg = 0, closed_fg = 1');
    $self->dbh->query('UPDATE events SET public_fg = 1, closed_fg = 0 WHERE id = 1');
    $self->dbh->query('UPDATE events SET public_fg = 0, closed_fg = 0 WHERE id = 2');
    $txn->commit();

    return $c->req->new_response(204, [], '');
};

post '/api/users' => [qw/allow_json_request/] => sub {
    my ($self, $c) = @_;
    my $nickname   = $c->req->body_parameters->get('nickname');
    my $login_name = $c->req->body_parameters->get('login_name');
    my $password   = $c->req->body_parameters->get('password');

    my ($user_id, $error);

    my $txn = $self->dbh->txn_scope();
    eval {
        my $duplicated = $self->dbh->select_one('SELECT * FROM users WHERE login_name = ?', $login_name);
        if ($duplicated) {
            $error = 'duplicated';
            $txn->rollback();
            return;
        }

        $self->dbh->query('INSERT INTO users (login_name, pass_hash, nickname) VALUES (?, SHA2(?, 256), ?)', $login_name, $password, $nickname);
        $user_id = $self->dbh->last_insert_id();
        $txn->commit();
    };
    if ($@) {
        $txn->rollback();
        warn "rollback by: $@";
        $error = 'unknown';
    }

    if ($error) {
        my $res = $c->render_json({
            error => $error,
        });
        $res->status(500);
        $res->status(409) if $error eq 'duplicated';
        return $res;
    }

    my $user = $self->get_user($user_id);
    delete $user->{recent_reservations};
    delete $user->{recent_events};

    my $res = $c->render_json($user);
    $res->status(201);
    return $res;
};

sub get_user {
    my ($self, $user_id) = @_;
    my $user = $self->dbh->select_row('SELECT * FROM users WHERE id = ?', $user_id);
    return unless $user;

    my @recent_reservations;
    {
        my $rows = $self->dbh->select_all('SELECT r.*, s.rank AS sheet_rank, s.num AS sheet_num FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id WHERE r.user_id = ? ORDER BY IFNULL(r.canceled_at, r.reserved_at) DESC LIMIT 5', $user_id);
        for my $row (@$rows) {
            my $event = $self->get_event($row->{event_id});
            $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
            $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
            delete $event->{sheets};
            delete $event->{total};
            delete $event->{remains};

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

        }
    };
    $user->{recent_reservations} = \@recent_reservations;
    $user->{total_price} = 0+$self->dbh->select_one('SELECT IFNULL(SUM(e.price + s.price), 0) FROM reservations r INNER JOIN sheets s ON s.id = r.sheet_id INNER JOIN events e ON e.id = r.event_id WHERE r.user_id = ? AND r.canceled_at IS NULL', $user_id);

    my @recent_events;
    {
        my $rows = $self->dbh->select_all('SELECT DISTINCT event_id FROM reservations WHERE user_id = ? ORDER BY IFNULL(canceled_at, reserved_at) DESC LIMIT 5', $user_id);
        for my $row (@$rows) {
            my $event = $self->get_event($row->{event_id});
            $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
            $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
            delete $event->{sheets}->{$_}->{detail} for keys %{ $event->{sheets} };

            push @recent_events => $event;
        }
    }
    $user->{recent_events} = \@recent_events;

    # sanitize fields
    delete $user->{login_name};
    delete $user->{pass_hash};
    return $user;
}

sub get_login_user {
    my ($self, $c) = @_;

    my $session = Plack::Session->new($c->env);
    my $user_id = $session->get('user_id');
    return unless $user_id;

    my $user = $self->get_user($user_id);
    delete $user->{recent_reservations};
    delete $user->{recent_events};
    return $user;
}

get '/api/users/{id}' => [qw/login_required/] => sub {
    my ($self, $c) = @_;

    my $user = $self->get_user($c->args->{id});
    if ($user->{id} != $self->get_login_user($c)->{id}) {
        my $res = $c->render_json({
            error => 'forbidden',
        });
        $res->status(403);
        return $res;
    }

    return $c->render_json($user);
};

post '/api/actions/login' => [qw/allow_json_request/] => sub {
    my ($self, $c) = @_;
    my $login_name = $c->req->body_parameters->get('login_name');
    my $password   = $c->req->body_parameters->get('password');

    my $user      = $self->dbh->select_row('SELECT * FROM users WHERE login_name = ?', $login_name);
    my $pass_hash = $self->dbh->select_one('SELECT SHA2(?, 256)', $password);
    if (!$user || $pass_hash ne $user->{pass_hash}) {
        my $res = $c->render_json({
            error => 'authentication_failed',
        });
        $res->status(401);
        return $res;
    }

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

    my $user = $self->get_login_user($c);
    my $event = $self->get_event($event_id, $user->{id});
    if (!$event || !$event->{public_fg}) {
        my $res = $c->render_json({
            error => 'not_found',
        });
        $res->status(404);
        return $res;
    }

    $event = $self->sanitize_event($event);
    return $c->render_json($event);
};

sub get_events {
    my $self = shift;

    my $txn = $self->dbh->txn_scope();

    my @events;
    my @event_ids = map { $_->{id} } @{ $self->dbh->select_all('SELECT id FROM events WHERE public_fg = 1 ORDER BY id ASC') };
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

        my $reservation = $self->dbh->select_row('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id, sheet_id HAVING reserved_at = MAX(reserved_at)', $event->{id}, $sheet->{id});
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

    return $event;
}

sub sanitize_event {
    my ($self, $event) = @_;
    my $sanitized = {%$event}; # shallow clone
    delete $sanitized->{price};
    delete $sanitized->{public_fg};
    return $sanitized;
}

post '/api/events/{id}/actions/reserve' => [qw/allow_json_request login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $rank = $c->req->body_parameters->get('sheet_rank');

    unless ($self->validate_rank($rank)) {
        my $res = $c->render_json({
            error => 'invalid_rank',
        });
        $res->status(400);
        return $res;
    }

    my $user  = $self->get_login_user($c);
    my $event = $self->get_event($event_id, $user->{id});
    unless ($event && $event->{public_fg}) {
        my $res = $c->render_json({
            error => 'invalid_event',
        });
        $res->status(404);
        return $res;
    }

    my $sheet;
    my $reservation_id;
    while (1) {
        $sheet = $self->dbh->select_row('SELECT * FROM sheets WHERE id NOT IN (SELECT sheet_id FROM reservations WHERE event_id = ? AND canceled_at IS NULL FOR UPDATE) AND `rank` = ? ORDER BY RAND() LIMIT 1', $event->{id}, $rank);
        unless ($sheet) {
            my $res = $c->render_json({
                error => 'sold_out',
            });
            $res->status(409);
            return $res;
        }

        my $txn = $self->dbh->txn_scope();
        eval {
            $self->dbh->query('INSERT INTO reservations (event_id, sheet_id, user_id, reserved_at) VALUES (?, ?, ?, ?)', $event->{id}, $sheet->{id}, $user->{id}, Time::Moment->now_utc->strftime('%F %T%f'));
            $reservation_id = $self->dbh->last_insert_id() + 0;
            $txn->commit();
        };
        if ($@) {
            $txn->rollback();
            warn "re-try: rollbacked by $@";
            next; # retry
        }

        last;
    }

    my $res = $c->render_json({
        reservation_id => $reservation_id,
        sheet_rank => $rank,
        sheet_num => $sheet->{num},
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
    unless ($event && $event->{public_fg}) {
        my $res = $c->render_json({
            error => 'invalid_event',
        });
        $res->status(404);
        return $res;
    }

    unless ($self->validate_rank($rank)) {
        my $res = $c->render_json({
            error => 'invalid_rank',
        });
        $res->status(404);
        return $res;
    }

    my $sheet = $self->dbh->select_row('SELECT * FROM sheets WHERE `rank` = ? AND num = ?', $rank, $num);
    unless ($sheet) {
        my $res = $c->render_json({
            error => 'invalid_sheet',
        });
        $res->status(404);
        return $res;
    }

    my $error;

    my $txn = $self->dbh->txn_scope();
    eval {
        my $reservation = $self->dbh->select_row('SELECT * FROM reservations WHERE event_id = ? AND sheet_id = ? AND canceled_at IS NULL GROUP BY event_id HAVING reserved_at = MAX(reserved_at) FOR UPDATE', $event->{id}, $sheet->{id});
        unless ($reservation) {
            $error = 'not_reserved';
            $txn->rollback();
            return;
        }
        if ($reservation->{user_id} != $user->{id}) {
            $error = 'not_permitted';
            $txn->rollback();
            return;
        }

        $self->dbh->query('UPDATE reservations SET canceled_at = ? WHERE id = ?', Time::Moment->now_utc->strftime('%F %T%f'), $reservation->{id});
        $txn->commit();
    };
    if ($@) {
        $txn->rollback();
        $error = 'unknown';
        warn "rollback by: $@";
    }

    if ($error) {
        my $res = $c->render_json({
            error => $error,
        });
        $res->status(500);
        $res->status(400) if $error eq 'not_reserved';
        $res->status(403) if $error eq 'not_permitted';
        return $res;
    }

    return $c->req->new_response(204, [], '');
};

sub validate_rank {
    my ($self, $rank) = @_;
    return unless $rank;

    my $valid = $self->dbh->select_one('SELECT COUNT(*) FROM sheets WHERE `rank` = ?', $rank);
    return $valid;
}

filter admin_login_required => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        my $session = Plack::Session->new($c->env);

        my $administrator = $self->get_login_administrator($c);
        unless ($administrator) {
            my $res = $c->render_json({
                error => 'admin_login_required',
            });
            $res->status(401);
            return $res;
        }

        $app->($self, $c);
    };
};

filter fillin_administrator => sub {
    my $app = shift;
    return sub {
        my ($self, $c) = @_;
        my $session = Plack::Session->new($c->env);

        if (my $administrator_id = $session->get('administrator_id')) {
            my $administrator = $self->get_administrator($administrator_id);
            $c->stash->{administrator} = $administrator;
        }

        $app->($self, $c);
    };
};

get '/admin/' => [qw/fillin_administrator/] => sub {
    my ($self, $c) = @_;

    my @events;
    if ($c->stash->{administrator}) {
        my @event_ids = map { $_->{id} } @{ $self->dbh->select_all('SELECT id FROM events') };
        for my $event_id (@event_ids) {
            my $event = $self->get_event($event_id);
            delete $event->{sheets}->{$_}->{detail} for keys %{ $event->{sheets} };
            $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
            $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
            push @events => $event;
        }
    }

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
    if (!$administrator || $pass_hash ne $administrator->{pass_hash}) {
        my $res = $c->render_json({
            error => 'authentication_failed',
        });
        $res->status(401);
        return $res;
    }

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

sub get_administrator {
    my ($self, $administrator_id) = @_;
    my $administrator = $self->dbh->select_row('SELECT * FROM administrators WHERE id = ?', $administrator_id);
    delete $administrator->{login_name};
    delete $administrator->{pass_hash};
    return $administrator;
}

sub get_login_administrator {
    my ($self, $c) = @_;

    my $session = Plack::Session->new($c->env);
    my $administrator_id = $session->get('administrator_id');
    return unless $administrator_id;
    return $self->get_administrator($administrator_id);
}

get '/admin/api/events' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;

    my @events;
    my @event_ids = map { $_->{id} } @{ $self->dbh->select_all('SELECT id FROM events') };
    for my $event_id (@event_ids) {
        my $event = $self->get_event($event_id);
        delete $event->{sheets}->{$_}->{detail} for keys %{ $event->{sheets} };
        $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
        $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
        push @events => $event;
    }

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
    $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
    $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
    return $c->render_json($event);
};

get '/admin/api/events/{id}' => [qw/admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};

    my $event = $self->get_event($event_id);
    unless ($event) {
        my $res = $c->render_json({
            error => 'not_found',
        });
        $res->status(404);
        return $res;
    }

    $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
    $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
    return $c->render_json($event);
};

post '/admin/api/events/{id}/actions/edit' => [qw/allow_json_request admin_login_required/] => sub {
    my ($self, $c) = @_;
    my $event_id = $c->args->{id};
    my $public = $c->req->body_parameters->get('public') ? 1 : 0;
    my $closed = $c->req->body_parameters->get('closed') ? 1 : 0;
    $public = 0 if $closed;

    my $event = $self->get_event($event_id);
    unless ($event) {
        my $res = $c->render_json({
            error => 'not_found',
        });
        $res->status(404);
        return $res;
    }

    if ($event->{closed_fg}) {
        my $res = $c->render_json({
            error => 'cannot_edit_closed_event',
        });
        $res->status(400);
        return $res;
    } elsif ($event->{public_fg} && $closed) {
        my $res = $c->render_json({
            error => 'cannot_close_public_event',
        });
        $res->status(400);
        return $res;
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
    $event->{public} = delete $event->{public_fg} ? JSON::XS::true : JSON::XS::false;
    $event->{closed} = delete $event->{closed_fg} ? JSON::XS::true : JSON::XS::false;
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

1;
