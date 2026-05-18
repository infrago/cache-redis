# cache-redis

`cache-redis` 是 `github.com/infrago/cache` 的**redis 驱动**。

## 包定位

- 类型：驱动
- 作用：把 `cache` 模块的统一接口落到 `redis` 后端实现

## 快速接入

```go
import (
    _ "github.com/infrago/cache"
    _ "github.com/infrago/cache-redis"
)
```

```toml
[cache]
driver = "redis"

[cache.setting]
addr = "127.0.0.1:6379"
timeout = "3s"
unlink = true
```

## `setting` 专用配置项

配置位置：`[cache].setting`

- `server`
- `addr`
- `username`
- `password`
- `database`
- `timeout`：单条 Redis 命令超时时间，默认 `3s`
- `unlink`：`Clear` 批量删除时使用 `UNLINK`，默认 `false` 使用 `DEL`

## 说明

- `setting` 仅对当前驱动生效，不同驱动键名可能不同
- 连接失败时优先核对 `setting` 中 host/port/认证/超时等参数
- `Keys` / `Clear` 使用 `SCAN` 分批扫描；这仍然是管理操作，不建议放在高频业务路径
- 驱动实现了 `SequenceMany`，批量取号会使用 Redis Lua 脚本原子完成
