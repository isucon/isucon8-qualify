use strict;
use warnings;
my $database  = $ENV{ISUCON8_DATABASE}  || 'isucon8_portal';
my $start_at  = $ENV{ISUCON8_START_AT}  || '2018-09-09T00:00:00+09:00';
my $finish_at = $ENV{ISUCON8_FINISH_AT} || '2018-09-15T00:00:00+09:00';
+{
    db => {
        dsn  => "DBI:mysql:database=$database;host=localhost;port=3306",
        user => 'isucon',
        pass => 'isucon',
        attr => {
            AutoCommit         => 1,
            RaiseError         => 1,
            ShowErrorStatement => 1,
            PrintWarn          => 0,
            PrintError         => 0,
            mysql_enable_utf8  => 1,
        },
    },
    game_time => {
        start_at  => $start_at,
        finish_at => $finish_at,
    },
};
