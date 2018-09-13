package ISUCON8::Portal;
use strict;
use warnings;
use utf8;
our $VERSION='0.01';
use ISUCON8::Portal::Container;

use feature 'state';

use Encode;
use Encode::UTF8Mac;
use Data::Dumper;
use Data::Validator::Recursive;
use Cwd qw(getcwd);

use parent qw/Amon2 Class::Accessor::Fast/;
# Enable project local mode.
__PACKAGE__->make_local_context();

sub _load_config {
    my ($self, %args) = @_;
    my $env = $args{env} || $ENV{PLACK_ENV};

    my $config = Amon2::Config::Simple->load($self, { environment => $env });

    ISUCON8::Portal::Container->register(
        'DB' => sub {
            my $c = shift;
            $c->load_class('ISUCON8::Portal::DB')->new(
                $config->{db}{dsn},
                $config->{db}{user},
                $config->{db}{pass},
                $config->{db}{attr},
            );
        },
        'Log' => sub {
            my $c = shift;
            my $cwd = getcwd;
            $c->load_class('ISUCON8::Portal::Log')->new(
                base_dir => "$cwd/log",
                pattern  => 'webapp.log.%Y%m%d.%H.%Y%m%d',
                symlink  => 'webapp.log',
                iomode   => '>>:unix',
                %{ $config->{log} },
            );
        },
    );

    ISUCON8::Portal::Container->instance->set_config($config);

    no warnings 'redefine';
    *load_config = sub { $config };
    *env         = sub { $env };
}

sub container {
    shift->{container} ||= ISUCON8::Portal::Container->instance;
}

sub json {
    my $c = shift;
    state $json = $c->container->get('JSON');
}

sub log {
    my $c = shift;
    state $log = $c->container->get('Log');
}

sub db {
    my $c = shift;
    state $db = $c->container->get('DB');
}

sub sql {
    my $c = shift;
    state $sql = $c->container->get('SQL');
}

sub model {
    my ($c, $model) = @_;
    $c->container->get('ISUCON8::Portal::Model::'.$model);
}

sub make_validator {
    my $c = shift;
    Data::Validator::Recursive->new(@_);
}

sub validate {
    my ($c, $validator, $params) = @_;
    my $args  = {};
    for my $rule (@{ $validator->{validator}->rules }) {
        my $key = $rule->{name};
        next unless exists $params->{ $rule->{name} };
        my $value = $params->{ $rule->{name} };
        next unless defined $value;
        my $type = $rule->{type}->{name};
        $args->{$key} = $c->trim($value, $type);
    }

    $validator->validate($args);
}

sub trim {
    my ($c, $val, $type) = @_;
    return $val unless defined $val;
    if (ref $val eq 'ARRAY') {
        for my $e (@$val) {
            $e = $c->trim($e, $type);
        }
    }
    elsif (ref $val eq 'HASH') {
        # TODO
    }
    else {
        $val =~ s/\A\s+|\s+\z//gms unless $type =~ /NoTrimWhiteSpace/;
        $val = decode 'utf-8-mac', encode_utf8 $val;
        $val =~ s/([[:cntrl:]])/$1 eq "\n" || $1 eq "\t" ? $1 : ""/msge;
    }

    return $val;
}

sub render_admin {
    my ($c, $tmpl, $args) = @_;
    $args //= {};
    $args->{is_admin} = 1;
    $c->render($tmpl, $args);
}

sub render_string {
    my $c = shift;
    local *Text::Xslate::render = Text::Xslate->can('render_string');
    $c->render(@_);
}

sub team {
    my $c = shift;
    $c->stash->{team} ||= do {
        my $team = $c->session->get('team');
        $team ? ISUCON8::Portal::Team->new($team) : undef;
    };
}

sub is_admin {
    my $c = shift;
    return $c->user->is_super_admin || $c->user->is_admin || 0;
}

### for DEBUG code
if (exists $INC{'Devel/KYTProf.pm'}) {
    Devel::KYTProf->mute('DBI::st', 'execute');
    Devel::KYTProf->ignore_class_regex(qr/Baran::Redis/);
    Devel::KYTProf->add_profs('RedisDB', ['send_command'], sub {
        my ($orig, $self, $command, @arguments) = @_;
        if ($self->{_in_multi}) {
            if ($command =~ /^(?:MULTI|EXEC|DISCARD)$/) {
                $command = "[$command]";
            }
            else {
                $command = q{  } . $command;
            }
        }
        return [
            @arguments ? '%s %s' : '%s',
            ['command', 'args'],
            {
                command => $command,
                args    => join(q{ }, @arguments),
            },
        ];
    });
    Devel::KYTProf->add_prof('RedisDB', '_connect', sub {
        my ($org, $self) = @_;
        if ($self->{path}) {
            return [
                'connect: %s',
                ['path'],
                {
                    path => $self->{path},
                },
            ];
        }
        else {
            return [
                'connect: %s:%s',
                ['host', 'port'],
                {
                    host => $self->{host},
                    port => $self->{port},
                },
            ];
        }
    });
}

sub _dump {
    my $c = shift;
    local $Data::Dumper::Purity = 1;
    local $Data::Dumper::Terse  = 1;
    local $Data::Dumper::Indent = 1;
    local $Data::Dumper::Useqq  = 1;
    warn Dumper @_;
}

1;
__END__

=head1 NAME

ISUCON8::Portal - ISUCON8::Portal

=head1 DESCRIPTION

This is a main context class for ISUCON8::Portal

=head1 AUTHOR

ISUCON8::Portal authors.

