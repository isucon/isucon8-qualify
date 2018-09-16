#!/usr/bin/env perl
use strict;
use warnings;
use autodie;
use utf8;
use feature 'say';
use Encode;
use Data::Dumper;
use File::Slurp qw(read_file write_file);

my $server_file = './data/vm_list_20180912.csv';
my $team_file   = './data/isucon8_team.tsv';

my $category_map = {
    '学生:1名' => 'student_one',
    '学生:2名' => 'student_two',
    '学生:3名' => 'student_three',
    '一般:1名' => 'general_one',
    '一般:2名' => 'general_two',
    '一般:3名' => 'general_three',
};

for my $setting (
    {
        team_out        => 'sql/isucon8q_day01_teams.sql',
        server_out      => 'sql/isucon8q_day01_servers.sql',
        team_member_out => 'sql/isucon8q_day01_team_members.sql',
        server_regexp   => qr/20180915/,
        team_regexp     => qr/9月15日/,
    },
    {
        team_out        => 'sql/isucon8q_day02_teams.sql',
        server_out      => 'sql/isucon8q_day02_servers.sql',
        team_member_out => 'sql/isucon8q_day02_team_members.sql',
        server_regexp   => qr/20180916/,
        team_regexp     => qr/9月16日/,
    },
) {
    my $servers = [ grep { $_ =~ $setting->{server_regexp} } read_file $server_file ];
    my $teams   = [ grep { $_ =~ $setting->{team_regexp}   } map { decode_utf8 $_ } read_file $team_file ];

    my $server_sqls = [];
    for my $line (@$servers) {
        my $server = [ split ',', $line =~ s/\r?\n$//r ];
        my (
            $group_id,
            $passowrd,
            $is_bench,
            $global_ip,
            $private_ip,
            $private_network,
            $bench_ip,
            $bench_network,
            $node,
        ) = @$server;

        my $hostname       = $global_ip =~ s/\./-/gr;
        my $is_bench_host  = $is_bench eq 'TRUE' ? 1 : 0;
        my $is_target_host = $private_ip =~ /1$/ ? 1 : 0;
        push @$server_sqls, encode_utf8 sprintf(
            "INSERT INTO servers VALUES('%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', '%s', %s, %s, %s, %s);\n",
            $group_id,
            $hostname,
            $passowrd,
            $global_ip,
            $private_ip,
            $private_network,
            $bench_ip,
            $bench_network,
            $node,
            $is_bench_host,
            $is_target_host,
            'UNIX_TIMESTAMP()',
            'UNIX_TIMESTAMP()',
        );
    }
    say "write $setting->{server_out}";
    write_file $setting->{server_out}, $server_sqls;

    my $teams_sqls        = [];
    my $team_members_sqls = [];
    for my $line (@$teams) {
        my $team = [ split "\t", $line =~ s/\r?\n$//r ];
        my (
            $id,
            $category,
            $name,
            $num,
            $m1,
            $m2,
            $m3,
            $passowrd,
            $group_id,
        ) = map { escape($_) } @$team[qw/1 2 4 5 6 7 8 9 10/];

        $category = $category_map->{"$category\:$num"};
        push @$teams_sqls, encode_utf8 sprintf(
            "INSERT INTO teams VALUES(%s, '%s', '%s', '%s', '%s', '%s', '', '', %s, %s);\n",
            $id,
            $group_id,
            'active',
            $name,
            $passowrd,
            $category,
            'UNIX_TIMESTAMP()',
            'UNIX_TIMESTAMP()',
        );

        my $member_number = 1;
        for my $member ($m1, $m2, $m3) {
            next unless length $member;
            push @$team_members_sqls, encode_utf8 sprintf(
                "INSERT INTO team_members VALUES('%s', '%s', '%s', '', '', %s, %s);\n",
                $id,
                $member_number++,
                $member,
                'UNIX_TIMESTAMP()',
                'UNIX_TIMESTAMP()',
            );
        }
    }
    say "write $setting->{team_out}";
    write_file $setting->{team_out}, $teams_sqls;

    say "write $setting->{team_member_out}";
    write_file $setting->{team_member_out}, $team_members_sqls;
}
sub escape {
    my $str = shift;
    $str =~ s/'/\\'/gr;
}
