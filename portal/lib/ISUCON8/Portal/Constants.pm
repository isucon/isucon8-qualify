package ISUCON8::Portal::Constants;

use strict;
use warnings;
use parent 'Exporter';
use Exporter::Constants ();

sub constants {
    my ($class, %stuff) = @_;
    my $pkg = caller(0);

    my $array = do {
        no strict 'refs';
        \@{"$pkg\::EXPORT"};
    };

    Exporter::Constants::_declare_constant($class, $array, \%stuff);
}

1;
