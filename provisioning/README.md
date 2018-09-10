# torb provisioning

`development` か `production` ファイルで指定されたホストにプロビジョニングする。

git管理外の `db/isucon8q-initial-dataset.sql.gz` もデプロイされる必要があるので、
実行前にローカルで生成しておく必要がある。


## プロビジョニング手順

```sh
$ go get github.com/constabulary/gb/... # 初回のみ必要

$ cd /path/to/torb

# db/isucon8q-initial-dataset.sql.gz の生成
$ cd bench
$ gb vendor restore
$ make
$ ./bin/gen-initial-dataset
$ cd ..

# プロビジョニングの実行
$ cd provisioning
$ vim development # or vim production
#=> デフォルトでは全部コメントアウトしているので、
#=> プロビジョニングしたいホストの部分のコメントアウトを外しておく。
$ ansible-playbook -i development site.yml # or ansible-playbook -i production site.yml
```

## メモ

- コメントアウトを外しているホストが、プロビジョニング対象になる。
    - デフォルトでは全部コメントアウトされているので、ansible-playbookを実行しても何も起きない。
- それぞれのロール(bench, webapp1, webapp2, webapp3)は、 `ロール名.yml` ファイルにそのロールで実行されるタスク一覧が書いてある。
    - ためしにあるタスクだけを実行したかったら、 `ロール名.yml` に書いてあるタスクをコメントアウトすることも可。
- それぞれのタスクが具体的に何をやっているのかは、 `タスク名/tasks/main.yml` に書いてある。
