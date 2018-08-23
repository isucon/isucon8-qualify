# project torb

this is a secret project.

see more details following:

- app/db/README.md
- app/perl/run_local.sh

### 環境構築

xbuildで言語をインストールします。ベンチマーカーのために、Goは必ずインストールしてください。 他の言語は使わないのであればスキップしても問題ありません。

```
cd
git clone https://github.com/tagomoris/xbuild.git

mkdir local
xbuild/perl-install   -f 5.28.0  /home/centos/local/perl
xbuild/go-install     -f 1.10.3  /home/isucon/local/go
```


### ベンチマーカーの準備

Goを使うのでこれだけは最初に環境変数を設定しておく

```
export PATH=$HOME/local/go/bin:$HOME/go/bin:$PATH
```

ビルド

```sh
go get github.com/constabulary/gb/...   # 初回のみ
cd bench
gb vendor restore
make
```

初期データ生成

```sh
cd bench
./bin/gen-initial-dataset   # isucon8q-initial-dataset.sql.gz ができる
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
./db/init.sh
```

### 参考実装(perl)を動かす

初回のみ

```
$ cd ~/torb/webapp/perl
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
