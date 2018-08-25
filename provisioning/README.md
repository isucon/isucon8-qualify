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
