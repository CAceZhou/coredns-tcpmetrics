# coredns-tcpmetrics

tcpmetrics 是一个 CoreDNS 外置插件。它在 Linux 主机上通过 NETLINK_INET_DIAG 和 TCP_INFO 读取 TCP socket 的内核统计数据，并通过带 Bearer Token 鉴权的 HTTPS API 输出给运维系统或 coredns-meshroute。

它提供的是主机 TCP 视角的重传率近似丢包率，而不是网卡、交换机或 Internet 链路上的真实物理丢包率。

## 功能

- 枚举 IPv4、IPv6 TCP socket，包含本地/远端地址、端口、状态和连接 ID。
- 输出发送段数、重传段数、首次/最后发现时间、RTT、拥塞窗口等 TCP_INFO 数据。
- 周期采样，能够处理短连接、连接消失和计数器变化。
- HTTPS API，所有请求使用 Bearer Token 鉴权，Token 采用常量时间比较。
- 支持按地址族、端口、CIDR、TCP 状态、正则和分页过滤连接。
- 非 Linux 系统保留相同 provider 接口，但会返回明确的当前平台不支持错误。

## 指标口径

loss_rate 的近似计算方式：

~~~text
loss_rate ≈ retransmits / sent_segments
~~~

它反映 TCP 端观察到的重传情况，可能受拥塞、对端响应、连接生命周期和采样窗口影响。短连接可能只在一次采样内出现；没有发送段的连接没有有意义的丢包率。TCP 重传不等于所有网络丢包，也可能来自重排、超时或对端行为。

## 前置条件

- Linux 内核；若要观测全机 socket，通常应以 root 身份运行。
- Go 1.25 或更新版本。
- CoreDNS 1.14.6（项目 CI 使用的版本）。
- HTTPS 证书和私钥；建议仅监听回环地址。

## 编译到 CoreDNS

这是外置插件，不能直接替换官方 CoreDNS 二进制。在 CoreDNS 源码树的 plugin.cfg 中、forward 之前添加：

~~~text
tcpmetrics:github.com/CAceZhou/coredns-tcpmetrics
~~~

然后执行：

~~~bash
go generate
go build -o coredns .
~~~

GitHub Actions 会自动测试插件并输出 Linux amd64、arm64 的包含本插件的 CoreDNS 二进制。若同时使用 meshroute，两个插件都必须注册到同一个 CoreDNS 源码树中。

## Corefile 配置

最小安全配置如下。此示例仅将 API 开放给本机：

~~~text
. {
    tcpmetrics 127.0.0.1:9165 {
        token 请替换为至少16字节的随机Token
        tls /etc/coredns/tls/node.crt /etc/coredns/tls/node.key
        sample 5s
        retain 30s
    }
    forward . 1.1.1.1
}
~~~

| 指令 | 必填 | 默认值 | 说明 |
| --- | --- | --- | --- |
| tcpmetrics <listen> | 是 | 无 | HTTPS 监听地址，例如 127.0.0.1:9165。 |
| token <value> | 是 | 无 | Bearer Token，至少 16 字节，禁止提交到 Git。 |
| tls <cert> <key> | 是 | 无 | PEM 格式服务端证书和私钥。SAN 必须匹配访问 API 的名称或 IP。 |
| sample <duration> | 否 | 5s | 采样周期。 |
| retain <duration> | 否 | 30s | 已消失连接的保留时间。 |
| allow_non_root | 否 | 关闭 | 允许非 root 启动；不保证能读取全部进程的 socket。 |

生产环境建议 API 保持在 127.0.0.1 或 ::1，由同机 meshroute 消费。远程访问必须额外配置防火墙、受信 CA、正确 SAN 和强随机 Token。

## HTTPS API

所有端点要求请求头：

~~~text
Authorization: Bearer <token>
~~~

| 端点 | 用途 |
| --- | --- |
| GET /v1/tcp/connections | 返回当前保留连接的分页列表。 |
| GET /v1/tcp/connections/{id} | 返回指定连接。 |
| GET /v1/tcp/summary | 返回连接数、已建立连接数、总发送/重传段数和加权重传率。 |

例如：

~~~bash
curl --cacert /etc/coredns/tls/ca.crt \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  'https://127.0.0.1:9165/v1/tcp/connections?state=ESTABLISHED&limit=100'
~~~

### 连接过滤

| 参数 | 示例 | 说明 |
| --- | --- | --- |
| family | 4、6 | 仅 IPv4 或 IPv6。 |
| state | ESTABLISHED | TCP 状态，不区分输入大小写。 |
| local_port | 25565,25566 | 本地端口集合。 |
| remote_port | 25565,25566 | 远端端口集合。 |
| local_cidr | 10.0.0.0/16 | 本地 IP 所属 CIDR。 |
| remote_cidr | 10.0.0.0/16 | 远端 IP 所属 CIDR。 |
| match | ^.*:25565.*$ | 匹配连接 ID、本地地址或远端地址的 Go 正则。 |
| offset | 0 | 偏移量，范围 0–1,000,000。 |
| limit | 100 | 每页数量，默认 100，最大 1000。 |

读取发往 10.0.0.0/16 中 25565/25566 的 IPv4 已建立连接：

~~~bash
curl --cacert ca.crt -H 'Authorization: Bearer YOUR_TOKEN' \
  'https://127.0.0.1:9165/v1/tcp/connections?family=4&state=ESTABLISHED&remote_cidr=10.0.0.0%2F16&remote_port=25565,25566'
~~~

典型字段包括 family、local_address、local_port、remote_address、remote_port、state、sent_segments、retransmits、loss_rate、rtt_us 和 congestion_window。

## 与 meshroute 配合

meshroute 使用本 API 的结构化过滤能力获取节点观测值。配置中的形式为：

~~~text
tcpmetrics https://127.0.0.1:9165 <token> <ca-file>
~~~

两者应使用同一个本机 CA，或提供能验证 tcpmetrics 证书的 CA 文件。不要让 meshroute 经公网地址访问本机 API。

## 运行与排障

启动后先检查摘要：

~~~bash
curl --fail --cacert /etc/coredns/tls/ca.crt \
  -H 'Authorization: Bearer YOUR_TOKEN' \
  https://127.0.0.1:9165/v1/tcp/summary
~~~

若连接列表为空，请依次确认：

1. CoreDNS 以 root 或具备读取 socket 所需能力的账户运行。
2. 主机存在 TCP 连接，且采样周期已经过去。
3. API 证书含 127.0.0.1 或所用 DNS 名称的 SAN。
4. Token 使用 Bearer 前缀，并与 Corefile 一致。
5. 过滤条件没有将目标连接排除。

## 开发与安全

~~~bash
go test ./...
go vet ./...
make check
~~~

- TCP 元数据可能泄漏服务地址、端口和连接模式，应将 API 视为敏感管理接口。
- 不要将 Token、私钥或生成的部署目录提交到仓库。
- 建议使用独立 CA，定期轮换证书和 Token。
- API 响应不可缓存；仍应避免在反向代理或日志中记录 Authorization 请求头。

本项目只做观测，不修改系统路由、不创建 VPN 隧道、不转发业务流量。
