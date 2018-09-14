+{
    db => {
        dsn  => 'DBI::mysql:database=isucon8_portal;host=localhost;port=3306',
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
};
