package ISUCON8::Portal::Web::ViewFunctions;
use strict;
use warnings;
use utf8;
use parent qw(Exporter);
use Module::Functions;
use File::Spec;
use Time::Piece;
use JavaScript::Value::Escape;

our @EXPORT = get_public_functions();

sub commify {
    local $_  = shift;
    1 while s/((?:\A|[^.0-9])[-+]?\d+)(\d{3})/$1,$2/s;
    return $_;
}

sub c { ISUCON8::Portal->context() }
sub uri_with { ISUCON8::Portal->context()->req->uri_with(@_) }
sub uri_for { ISUCON8::Portal->context()->uri_for(@_) }

sub is_active {
    $_[0] eq $_[1] ? 'is-active' : '';
}

sub unixtime2time {
    my $unixtime = shift || return '';
    localtime($unixtime)->strftime("%H:%M:%S")
}

sub from_unixtime {
    my $unixtime = shift;
    localtime($unixtime)->strftime("%Y-%m-%d %H:%M:%S")
}

sub ellipsis {
    my ($str, $max_length) = @_;
    return $str unless length $str > $max_length;
    return substr($str, 0, $max_length - 3) . '...';
}

sub json {
    javascript_value_escape(
        JSON::XS->new->utf8->encode(@_)
    );
}

sub html_line_break  {
    my $text = shift;
    $text = Text::Xslate::Util::html_escape($text);
    $text =~ s|(\r?\n)|<br />$1|g;
    return Text::Xslate::mark_raw($text);
}

{
    my %static_file_cache;
    sub static_file {
        my $fname = shift;
        my $c = ISUCON8::Portal->context;
        if (not exists $static_file_cache{$fname}) {
            my $fullpath = File::Spec->catfile($c->base_dir(), $fname);
            $static_file_cache{$fname} = (stat $fullpath)[9];
        }
        return $c->uri_for(
            $fname, {
                't' => $static_file_cache{$fname} || 0
            }
        );
    }
}

1;
