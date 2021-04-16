## N9E-TSDB

### 接入方式
下载安装包
```
wget 116.85.64.82/n9e-tsdb.tar.gz
```

需要修改server.yml的配置如下
```
transfer:
  enable: true
  backend:
    datasource: "tsdb"
    m3db:
      enabled: false #修改为false
      maxSeriesPoints: 720 # default 720
      name: "m3db"
      namespace: "default"
      seriesLimit: 0
      docsLimit: 0
      daysLimit: 7                               # max query time
      # https://m3db.github.io/m3/m3db/architecture/consistencylevels/
      writeConsistencyLevel: "majority"          # one|majority|all
      readConsistencyLevel: "unstrict_majority"  # one|unstrict_majority|majority|all
      config:
        service:
          # KV environment, zone, and service from which to write/read KV data (placement
          # and configuration). Leave these as the default values unless you know what
          # you're doing.
          env: default_env
          zone: embedded
          service: m3db
          etcdClusters:
            - zone: embedded
              endpoints:
                - 127.0.0.1:2379
              tls:
                caCrtPath: /etc/etcd/certs/ca.pem
                crtPath: /etc/etcd/certs/etcd-client.pem
                keyPath: /etc/etcd/certs/etcd-client-key.pem
    tsdb:
      enabled: true
      name: "tsdb"
      cluster:
        tsdb01: 127.0.0.1:8011

monapi:
  indexMod: index
  alarmEnabled: true
  region:
    - default

judge:
  query:
    connTimeout: 1000
    callTimeout: 2000
    maxConn:          2000
    maxIdle:          100
    connTimeout:      1000
    callTimeout:      2000
    indexCallTimeout: 2000
    indexMod:         index

```

在etc/address.yml中，添加tsdb和index的配置
```
tsdb:
  http: 0.0.0.0:8010
  rpc: 0.0.0.0:8011
  addresses:
    - 127.0.0.1

index:
  http: 0.0.0.0:8012
  rpc: 0.0.0.0:8013
  addresses:
    - 127.0.0.1
```

## 修改nginx配置
原来的配置：
```
location /api/index {
    proxy_pass http://n9e.server;
}
```

改成：
```
location /api/index {
    proxy_pass http://n9e.index;
}
```

### 编译
```
./control build
```

### 启动
```
./control start all
```