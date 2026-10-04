# 泄洪闸门操作协调服务

## 项目目标

该服务用于水工控制室协调泄洪闸门的命令申请、安全联锁复核和单次开度调整。系统在同一闸门上只允许一条已通过复核且尚未结束的执行命令，任何冲突都必须被显式拒绝，不能覆盖已经占用的闸门。

## 使用角色

- 运行协调员：登记闸门并提交开度调整命令。
- 安全复核员：提交联锁结论，决定命令是否通过。
- 闸门操作员：凭执行令牌启动命令并登记执行结果。

## 核心实体

- `gate`：闸门及其当前开度、运行状态、最大开度和活动命令。
- `gate_command`：一次开度调整申请，包含目标开度、申请原因、状态和执行令牌。
- `interlock_decision`：安全复核结论，状态为 clear 或 blocked；结论上固化当时生效规则版本的不可变依据快照（rule basis）。
- `interlock_rule_set`：联锁规则的一个已发布版本，拥有严格单调的生效边界、版本号和内容哈希。规则链只追加、不回写。

## 业务流程

### register-gate

运行协调员通过 `POST /gates` 登记闸门。服务校验标识、名称、最大开度和初始开度，写入空闲状态；重复标识返回稳定冲突错误，不覆盖原记录。

### request-gate-command

运行协调员通过 `POST /commands` 提交开度调整命令。服务读取闸门当前开度和限制，校验目标开度与申请信息，创建待复核命令并冻结起始开度。

### review-safety-interlock

安全复核员通过 `POST /commands/{id}/interlock` 提交复核结论。复核在同一临界区内解析当前有效规则版本：clear 结论必须同时满足当时可读且生效的规则（例如汛期最大允许开度），满足时原子占用闸门并签发一次执行令牌；blocked 结论会把命令置为 rejected。其他命令先占用同一闸门时，当前 clear 结论返回冲突且不改变闸门。

当 clear 结论违反当前规则时，复核返回 `interlock_clear_forbidden_by_rules` 冲突，命令保持 pending，不写入结论、令牌或闸门占用——因此同一条待处理命令在规则修订前后重新提交都能得到稳定且可解释的结果。通过或驳回的结论都会把当时生效的规则版本快照（版本号、规则标识、生效边界、内容哈希）固化在 `interlock.basis` 上，后续规则修订不会回写这一依据。

### publish-interlock-rules

安全管理员通过 `POST /rule-sets` 发布新版本联锁规则。新版本必须携带严格晚于当前最新边界的生效时间（缺省为当前时刻），版本号由服务按链连续分配，标识由服务生成，内容哈希覆盖边界与规则体。新版本只追加：不能回插过去边界、不能替换旧版本、也不影响已经形成的占用、终态或历史结论。

### resolve-effective-rules

通过 `GET /rule-sets` 读取完整版本链（含每个版本的 `effective_to` 上界和当前 `active` 标记），通过 `GET /rule-sets/effective?at=<RFC3339>` 解析任一时刻有效且可读的版本，通过 `GET /rule-sets/{id}` 按标识读取单个版本。

### execute-gate-command

闸门操作员通过 `POST /commands/{id}/execute` 提交执行令牌。服务校验命令、令牌、闸门占用关系和目标开度，在临界区内完成 approved、executing、completed 或 aborted 的转换，并同步更新闸门状态。令牌在命令结束后失效，不能复用。

## 状态与规则

- `gate` 状态：idle、reserved、moving、faulted。
- `gate_command` 状态：pending、approved、rejected、executing、completed、aborted。
- 只有 pending 命令可以复核。
- 只有 approved 命令可以执行，且执行令牌必须匹配。
- clear 复核与闸门占用必须同时成功；任一步失败时两者都不写入。
- blocked 复核使命令 rejected，不会占用闸门。
- 模拟故障会令命令 aborted、闸门 faulted，并保留最终开度。
- 已完成或已中止的命令不能再次执行。
- 联锁规则版本链只追加：基线版本在零时刻生效，后续版本的生效边界必须严格晚于上一版本，不能回插、不能覆盖。
- 复核在仓库写锁内解析当时有效的规则版本并把依据快照固化到结论；规则版本只在复核时刻读取一次，修订发布与复核不会互相交错。
- clear 结论必须满足当时规则版本的开度上限；违反时命令保持 pending，不产生任何写入，复核可在修订后安全重试。
- 规则修订不回写既有占用、终态命令或历史结论的依据快照。

## 模块边界

- `cmd/server`：进程入口、监听地址和退出控制。
- `internal/httpapi`：HTTP 路由、JSON 解码、状态码映射。
- `internal/coordination`：工作流编排、命令标识和执行令牌签发。
- `internal/domain`：实体、不变量和状态转换规则。
- `internal/storage`：带互斥保护的内存仓库和原子写入。
- `internal/contracts`：API 请求、响应和稳定错误结构。
- `internal/bootstrap`：运行时依赖装配。

## 接口

- `GET /health`
- `POST /gates`
- `GET /gates/{id}`
- `POST /commands`
- `GET /commands/{id}`
- `POST /commands/{id}/interlock`
- `POST /commands/{id}/execute`
- `POST /rule-sets`
- `GET /rule-sets`
- `GET /rule-sets/effective?at=<RFC3339>`
- `GET /rule-sets/{id}`

错误响应统一为：

```json
{"code":"stable_error_code","message":"可读说明"}
```

## 验证计划

每条工作流通过 `cmd/workflowcheck` 启动真实 HTTP 服务并调用公开接口，校验成功状态、持久化结果和关键失败分支。`interlock-rule-versioning` 工作流验证基线种子、版本发布与未来边界、按时刻解析有效版本、clear 违规不写入、blocked 依据固化、历史结论不被回写、以及回插边界被稳定拒绝。单元测试覆盖规则版本不变量、仓库边界解析和服务层发布校验。
