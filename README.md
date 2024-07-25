# tke-workload-metrics

## 配置文件

配置集群 ID、命名空间、地域、云 API 凭证等信息，保存为 `.metrics/config.yaml`，格式如下：

``` yaml
# .metrics/config.yaml
region: ap-guangzhou
clusterID: cls-xxx
namespace: default
secretID: 
secretKey: 
```

## 如何运行

```shell
$ ./tke-workload-metrics --help
```
