use strict;
use warnings;
use Time::Moment;
my $database  = $ENV{ISUCON8_DATABASE}  || 'isucon8_portal';
my $start_at  = $ENV{ISUCON8_START_AT}  || '2018-09-15T10:00:00+09:00';
my $finish_at = $ENV{ISUCON8_FINISH_AT} || '2018-09-15T18:00:00+09:00';

+{
    db => {
        dsn  => "DBI:mysql:database=$database;host=localhost;port=3306",
        user => 'isucon',
        pass => 'isucon',
        attr => {
            AutoCommit           => 1,
            RaiseError           => 1,
            ShowErrorStatement   => 1,
            PrintWarn            => 0,
            PrintError           => 0,
            mysql_enable_utf8    => 1,
            mysql_enable_utf8mb4 => 1,
        },
    },
    contest_period => {
        start_at  => Time::Moment->from_string($start_at)->epoch,
        finish_at => Time::Moment->from_string($finish_at)->epoch,
    },
    url => {
        isucon_site => 'http://isucon.net/',
        regulation  => 'http://isucon.net/archives/52445389.html',
        twitter     => 'https://twitter.com/isucon_official',
    },
};
