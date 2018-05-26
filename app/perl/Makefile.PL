use ExtUtils::MakeMaker;

WriteMakefile(
    NAME          => 'Torb',
    VERSION_FROM  => 'lib/Torb.pm',
    PREREQ_PM     => {
        'Kossy' =>  '0.40',
    },
    MIN_PERL_VERSION => '5.008001'
);

