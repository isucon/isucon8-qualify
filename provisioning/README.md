# torb provisioning

```sh
$ cd /path/to/torb
$ cd provisioning

$ vim development
#=> edit target hosts

$ ansible-playbook -i development site.yml
```
