# 泄洪闸门操作协调服务

这是一个无外部依赖的 Go 后端，用本地内存仓库协调闸门登记、开度调整命令、安全联锁复核和执行结果。服务强调单闸门活动命令互斥、原子状态转换、执行令牌生命周期和稳定错误传播。

## 运行

```sh
go build ./...
go run ./cmd/server --addr 127.0.0.1:18119
```

健康入口为 `GET http://127.0.0.1:18119/health`。

## API

```text
POST /gates
GET  /gates/{id}
POST /commands
GET  /commands/{id}
POST /commands/{id}/interlock
POST /commands/{id}/execute
POST /rule-sets
GET  /rule-sets
GET  /rule-sets/effective
GET  /rule-sets/{id}
```

登记闸门示例：

```json
{
  "id": "gate-east",
  "name": "东侧泄洪闸",
  "max_aperture": 4.5,
  "initial_aperture": 0.5
}
```

提交命令示例：

```json
{
  "gate_id": "gate-east",
  "target_aperture": 2.25,
  "reason": "上游水位调整",
  "requested_by": "dispatcher-lin"
}
```

联锁复核示例：

```json
{
  "verdict": "clear",
  "verifier": "safety-wu",
  "note": "上下游条件满足"
}
```

执行示例：

```json
{
  "operator": "operator-chen",
  "token": "复核响应中的 execution_token",
  "simulate_fault": false
}
```

### 联锁规则版本

服务启动时自动播种一条永久有效的基线规则版本（`rule-baseline`，revision 1）。安全管理员可以发布新版本，规则链只追加：

```sh
# 立即生效的汛期版本（收紧最大允许开度）
curl -X POST http://127.0.0.1:18119/rule-sets \
  -H 'Content-Type: application/json' \
  -d '{"max_allowed_aperture": 2.0, "season": "flood-season", "note": "汛期收紧开度"}'

# 预约未来时刻生效的版本（effective_from 为 RFC3339）
# 缺省 effective_from 表示立即生效
```

读取版本链、按标识读取单个版本、按时刻解析有效版本：

```text
GET /rule-sets
GET /rule-sets/effective
GET /rule-sets/effective?at=2026-07-01T00:00:00Z
GET /rule-sets/{id}
```

列表中的每个版本带 `revision`、`effective_from`、`effective_to`、`active` 和内容 `content_hash`。

复核命令时，服务在同一临界区内解析当时可读且生效的版本，并把依据快照写入结论。命令的 `interlock.basis` 会记录：

```json
{
  "verdict": "clear",
  "verifier": "safety-wu",
  "basis": {
    "rule_set_id": "rule-xxxx",
    "revision": 2,
    "season": "flood-season",
    "effective_from": "2026-06-01T...",
    "max_allowed_aperture": 2,
    "content_hash": "..."
  }
}
```

关键语义：

- 规则版本的生效边界必须严格晚于上一版本；回插过去边界返回 `rule_effective_boundary_conflict`，不会覆盖旧版本。
- clear 结论若违反当时生效版本（目标开度超过 `max_allowed_aperture`），返回 `interlock_clear_forbidden_by_rules`，命令保持 pending，不占用闸门、不签发令牌、不写入结论，可在规则修订后安全重试或改判 blocked。
- 已通过复核的命令及其依据快照不受后续修订影响；执行阶段不重新评估规则。
- blocked 结论同样固化当时的规则依据，安全人员可凭 `interlock.basis` 说明任何历史结论采用的是哪一版规则。

## 目录

- `cmd/server`：服务进程。
- `cmd/workflowcheck`：五条工作流的真实 HTTP 检查入口。
- `internal/bootstrap`：依赖装配（含基线规则种子）。
- `internal/contracts`：公开数据契约。
- `internal/coordination`：业务工作流与规则版本发布。
- `internal/domain`：实体、不变量、状态机和规则版本链。
- `internal/httpapi`：路由、解码和错误映射。
- `internal/storage`：互斥保护的内存仓库、原子写入和有效版本解析。

## 工作流检查

```sh
go run ./cmd/workflowcheck --workflow register-gate
go run ./cmd/workflowcheck --workflow request-gate-command
go run ./cmd/workflowcheck --workflow review-safety-interlock
go run ./cmd/workflowcheck --workflow execute-gate-command
go run ./cmd/workflowcheck --workflow interlock-rule-versioning
```

这些检查会创建临时 HTTP 服务并调用真实路由。它们不调用外部网络，也不依赖数据库。

## 测试

单元测试位于 `internal/domain`、`internal/storage` 和 `internal/coordination`：

```sh
go test ./...
```
