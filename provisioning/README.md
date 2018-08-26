# torb provisioning

ローカルのtorbリポジトリの内容を、 `development` ファイルに指定したデプロイする。


```sh
$ cd /path/to/torb
$ cd provisioning

$ vim development
#=> デフォルトでは全部コメントアウトしているので、
#=> 設定をデプロイしたいホストの部分のコメントアウトを外しておく。

$ ansible-playbook -i development site.yml
#=> デプロイされる
```

---

- `development` でコメントアウトを外しているホストが、デプロイ対象になる。
    - デフォルトでは全部コメントアウトされているので、ansible-playbookを実行しても何も起きない。
- それぞれのロール(bench, webapp1, webapp2, webapp3)は、 `ロール名.yml` ファイルにそのロールで実行されるタスク一覧が書いてある。
    - ためしにあるタスクだけを実行したかったら、 `ロール名.yml` に書いてあるタスクをコメントアウトすることも可。
- それぞれのタスクが具体的に何をやっているのかは、 `タスク名/tasks/main.yml` に書いてある。
