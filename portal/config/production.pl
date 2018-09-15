use strict;
use warnings;
use Time::Moment;
my $database  = $ENV{ISUCON8_DATABASE}  || 'isucon8_portal';
my $start_at  = $ENV{ISUCON8_START_AT}  || '2018-09-15T10:00:00+09:00';
my $finish_at = $ENV{ISUCON8_FINISH_AT} || '2018-09-15T18:00:00+09:00';

# 9/15
# my $manual_url  = 'https://gist.github.com/rkmathi/04d02d5fd95ddcf2a9d59ae2b5d79432';
# my $discord_url = 'https://discordapp.com/channels/484181541476368393/489669006387838976';

# 9/16
my $manual_url  = 'https://gist.github.com/rkmathi/1d08e17671d3952e8d2e873e686b7ea6';
my $discord_url = 'https://discordapp.com/channels/484182004989034496/484182004989034498';

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
        discord     => $discord_url,
        manual      => $manual_url,
        isucon_site => 'http://isucon.net/',
        regulation  => 'http://isucon.net/archives/52445389.html',
        twitter     => 'https://twitter.com/isucon_official',
    },
};
