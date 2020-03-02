# ISUCON8 AMI作成
* CentOS7.5
* Ansibleの適応までを終わらせたAMIを作る
* Packerのビルド長いのでベースとなるCentOS7.5のイメージを先に作る

## 構築
* ベースCentOSイメージ作成
`packer build -parallel=false -var aws_profile=**** base_centos.json`
* ISUCON8イメージ作成
`packer build -parallel=false -var aws_profile=**** -var ami_id=** isucon8.json`
