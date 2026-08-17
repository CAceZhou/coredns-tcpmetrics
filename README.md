# coredns-tcpmetrics

`tcpmetrics` is an out-of-tree CoreDNS plugin that collects per-socket Linux TCP statistics through `NETLINK_INET_DIAG`/`TCP_INFO` and publishes them through a Bearer-authenticated HTTPS API.

Loss rate is estimated from retransmitted and sent TCP segment counters. It is not a link-layer loss measurement.

## Build with CoreDNS

Add the plugin before `forward` in CoreDNS `plugin.cfg`:

```text
tcpmetrics:github.com/CAceZhou/coredns-tcpmetrics
```

Then run `go generate` and `go build` in the CoreDNS repository.

## Corefile

```text
tcpmetrics 127.0.0.1:9165 {
    token replace-with-at-least-16-random-bytes
    tls /etc/coredns/node.crt /etc/coredns/node.key
    sample 5s
    retain 30s
    # allow_non_root
}
```

Linux normally requires root or the capability needed to inspect all sockets. `allow_non_root` permits startup without root but does not guarantee visibility into every process.

## API

Every request must use HTTPS and `Authorization: Bearer <token>`:

```text
GET /v1/tcp/connections?state=ESTABLISHED&match=443&offset=0&limit=100
GET /v1/tcp/connections/{id}
GET /v1/tcp/summary
```

Connection queries also support exact structured filters:

```text
family=4|6
local_port=25565,25566
remote_port=25565,25566
local_cidr=10.0.0.0/16
remote_cidr=10.0.0.0/16
```

Every connection includes a `family` field in addition to its endpoints and TCP_INFO counters.

The exported `telemetry` package provides the shared in-process store consumed by `coredns-meshroute` connection rules.

## Development

Go 1.25 or newer is required.

```bash
make check
```
