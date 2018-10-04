# ISUCON8 予選問題

* [予選レギュレーション](./doc/REGULATION.md)
* [予選マニュアル](./doc/MANUAL.md)

## 本番のマシンスペック

* vCPU 2コア : Intel(R) Xeon(R) CPU E5-2640 v4 @ 2.40GHz
* メモリ 1GB
* ネットワーク帯域 1Gbps
* ディスク SSD

のVMを競技用に３台。同スペックのVMをベンチマーカー用に1台。

## 本番のOS/初期ミドルウェア

* CentOS 7.5.1804
* MariaDB 5.5.60
* H2O 2.2.5

## 感想戦用、1VMでの動かし方

環境構築の詳細については [provisioning](./provisioning) を参照。

### 環境構築

xbuildで言語をインストールする。ベンチマーカーのためにGoは必須。他の言語は使わないのであればスキップしても問題ない。

```
cd
git clone https://github.com/tagomoris/xbuild.git

mkdir local
xbuild/go-install     1.10.3  $HOME/local/go
xbuild/perl-install   5.28.0  $HOME/local/perl
xbuild/ruby-install   2.5.1   $HOME/local/ruby
xbuild/node-install   v8.11.4 $HOME/local/node
xbuild/python-install 3.7.0   $HOME/local/python
xbuild/php-install    7.2.9   $HOME/local/php -- --with-pcre-regex --with-zlib --enable-fpm --enable-pdo --with-pear --with-mysqli=mysqlnd --with-pdo-mysql=mysqlnd --with-openssl --with-pcre-regex --with-pcre-dir --with-libxml-dir --enable-opcache --enable-bcmath --with-bz2 --enable-calendar --enable-cli --enable-shmop --enable-sysvsem --enable-sysvshm --enable-sysvmsg --enable-mbregex --enable-mbstring --enable-pcntl --enable-sockets --with-curl --enable-zip
```

### ベンチマーカーの準備

Goを使うのでこれだけは最初に環境変数を設定しておく

```
export PATH=$HOME/local/go/bin:$HOME/go/bin:$PATH
```

ビルド

```sh
$ cd bench
$ make deps
$ make
```

初期データ生成

```sh
$ cd bench
$ ./bin/gen-initial-dataset   # ../db/isucon8q-initial-dataset.sql.gz ができる
```

### データベース初期化

データベース初期化、アプリが動くのに最低限必要なデータ投入

```sh
$ mysql -uroot
mysql> CREATE USER isucon@'%' IDENTIFIED BY 'isucon';
mysql> GRANT ALL on torb.* TO isucon@'%';
mysql> CREATE USER isucon@'localhost' IDENTIFIED BY 'isucon';
mysql> GRANT ALL on torb.* TO isucon@'localhost';
```

```
$ ./db/init.sh
```

### 参考実装(perl)を動かす

初回のみ

```
$ cd webapp/perl
$ cpanm --installdeps --notest --force .
```

起動

```
$ ./run_local.sh
```

### ベンチマーク実行

```console
$ cd bench
$ ./bin/bench -h # ヘルプ確認
$ ./bin/bench -remotes=127.0.0.1:8080 -output result.json
```

結果を見るには `sudo apt install jq` で jq をインストールしてから、

```
$ jq . < result.json
```

## 既知の不具合

- 管理画面から新規イベントを作成する際に、ベース金額を初期値から変更した場合、金額を数値型ではなく文字列型で POST してしまう JS のバグがありました。 初期実装を確認したところ、このバグに影響を受けるのは go 実装のみでした。また、ベンチマークには影響がないことが分かっているため、JS の修正は見送りました。 なお、JS を書き換えるとベンチマークが fail しますので JS の改修はお勧めしません。
- 変数名等で使われている、座席を意味する単語 **sheets** の、正しいスペルは **seats** です。
