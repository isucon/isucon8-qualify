# torb provisioning

## ポータル、ベンチマーカ、競技用webappの初期デプロイをする君

### プロビジョニング手順

```sh
$ go get -v github.com/constabulary/gb/... # 初回のみ必要

$ cd /path/to/torb/provisioning
$ vim development
#=> プロビジョニングしたいホストの部分のコメントアウトを外しておく。

$ vim roles/prepare_bench/files/torb.bench.service
#=> -portal http://127.0.0.1 となっているがコレだとbenchからみたportalが127.0.0.1に
#=> なってしまうので、もしもportalを他のIPに撒いているならそれに書き換える。

$ ansible-playbook -i development site.yml
```

### メモ

- コメントアウトを外しているホストが、プロビジョニング対象になる。
    - デフォルトでは全部コメントアウトされているので、ansible-playbookを実行しても何も起きない。
- それぞれのロール(portal_web, bench, webapp1, webapp2, webapp3)は、 `ロール名.yml` ファイルにそのロールで実行されるタスク一覧が書いてある。
    - ためしにあるタスクだけを実行したかったら、 `ロール名.yml` に書いてあるタスクをコメントアウトすることも可。
- それぞれのタスクが具体的に何をやっているのかは、 `タスク名/tasks/main.yml` に書いてある。

---

## portalをデプロイ・kill -HUPする君

```sh
$ cd /path/to/torb/provisioning

# ステージング環境
$ vim development
#=> [portal_web]のブロックで指定されているホストのコメントアウトを外す
$ time ansible-playbook -i development portal_deploy_and_sighup.yml

# 本番環境
$ vim production
#=> [portal_web]のブロックで指定されているホストのコメントアウトを外す
$ time ansible-playbook -i production portal_deploy_and_sighup.yml
```

---

## benchのバイナリをデプロイする君

```sh
# /path/to/torb/bench/bin.Linux.x86_64/bench にLinux用のbenchのバイナリを置いておく

$ cd /path/to/torb/provisioning

# ステージング環境
$ vim development
#=> [bench]のブロックで指定されているホストのコメントアウトを外す
$ time ansible-playbook -i development deploy_bench_binary.yml

# 本番環境
$ vim production
#=> [bench]のブロックで指定されているホストのコメントアウトを外す
$ time ansible-playbook -i production deploy_bench_binary.yml
```
