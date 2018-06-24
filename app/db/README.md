# setup

```bash
mysqladmin -uroot create torb
mysql -uroot torb < schema.sql
mysql -uroot torb < init.sql
```
