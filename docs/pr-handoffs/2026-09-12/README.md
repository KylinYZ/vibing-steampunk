# vibing-steampunk 独立 PR 修复交接入口

形成日期：2026-09-12。任务：核对 TypeScript 参考仓库是否已有相关方案，并为后续逐会话、逐分支实施生成交接。本次只新增本文档目录；未修改业务代码，未创建修复分支、提交或 PR，未执行真实 SAP 或数据库操作。

## 先读结论

“有参考实现”不等于“Go 问题已经解决”，也不等于可直接复制整个工作流。按协议实现、设计模式、测试证据分别判断：

| 顺序 | 任务 | TypeScript 仓库的对应情况 | 建议 |
|---|---|---|---|
| 1 | [#169 自动加锁写入与包校验](./01-169-self-lock-session.md) | 有会话隔离/锁内写入模式；没有 Go 包校验 marker 的同构补丁 | 优先；Go 自身已有 PrepareSourceUpdate，应局部复用 |
| 2 | [#153 DTEL 信息读取](./02-153-data-element-read.md) | 有实际 DDIC XML 字段解析、公开读取入口及历史 DEV 复读证据 | 最小独立 PR；同时修 Accept 与解析 |
| 3 | [#166 失败解锁与清理结果](./03-166-lock-cleanup.md) | 有主错误/解锁错误记录、未知结果停止、删除后查缺失 | 借鉴状态语义；Go 的独立取消上下文须用自身实现 |
| 4 | [#117 CLI 安全配置](./04-117-cli-safety-config.md) | 无 Cobra/同款 flag 方案；只有配置解析/策略门控参考 | Go 原生修复；不能只改 PersistentFlags |
| 5 | [#112 cookie 文件刷新](./05-112-cookie-file-refresh.md) | 没有 cookie 文件热重载；有单次只读恢复、并发合并和禁止写重放 | 需新实现；恢复范围必须显式受限 |
| 6 | [#143 现有对象更新免传 package](./06-143-existing-object-package.md) | 有对象元数据解析后校验的模式；没有同款包 allowlist 机制 | 单独 PR；与 #169 相邻但非同一缺陷 |
| 已有 PR | [#109 / PR #192 DOMA、DTEL 创建](./07-109-192-ddic-creation.md) | 有完整创建、属性、激活、回读及历史生命周期证据 | 继续原 PR，不另开重复实现 |
| 后续功能 | [#74 DDLX 元数据扩展](./08-74-ddlx.md) | 有明确创建协议、适配器、测试与历史 DEV 证据 | 可移植，但只借鉴创建部分不代表已实现 update/upsert |
| 暂缓 | [DAP、OAuth2、最小安装器](./09-deferred-directions.md) | 见分项边界 | 不混入上述修复 |

排序是本次建议，不代表维护者已指派或同意合并。上轮在线核对：上游 main 与本地均为 21d2f22；#222 是分诊报告 PR，当时仅新增一份报告，其“后续修复将落在此分支”不表示修复已经存在。#221、#217、#192 等为在途 PR，开工前须重新检查。

## 两个仓库的事实基线

- 修复目标：[vibing-steampunk](D:/MyDev/SAP/vibing-steampunk)，HEAD 21d2f22ec6de4b43f18ff10b2e0d53bfa875ff6b。
- 只读参考：[mcp-abap-abap-adt-api](D:/MyDev/SAP/mcp-abap-abap-adt-api)，HEAD a8cdeda38dc4bbb8a98896e4f42e5925eed3d8ef，package.json 为 0.7.0。
- 参考仓库 AGENTS.md 尚写 0.6.0；本次以当前 package.json/源码为基线，未顺手更新原有文档。
- 参考仓库有既存未提交修改。本文引用的 RepositoryObjectCleanupWorkflow.ts、RepositoryObjectCreationWorkflow.ts 及相关策略/集成文件可能包含工作区增量；以 [baseline.json](./baseline.json) 的路径、SHA256 和工作区状态为准，不能单凭 HEAD 复现所有内容。
- 目标仓库原有未跟踪项为 .claude/settings.local.json、.codegraph/；不要纳入业务 PR。
- 本目录是本地跨仓库交接材料，带绝对路径。不要未经审阅将整目录加入上游 PR；上游 PR 只提交该问题的代码、测试及适当的公开说明。
- 历史 DEV 文档只用于证明参考实现曾验证哪些协议。本次没有复测，也不复用历史对象、传输号、计划或凭据。

## 独立会话与分支的工作方式

每个任务页末尾有可直接粘贴的新会话提示词。默认从开工时更新后的 upstream/main 创建隔离 worktree；分支名只是建议，本轮尚未创建。

1. 读取本入口与单项任务页；核实当前目录、HEAD、工作区和适用规则。
2. 在线复核 issue、评论、关联 PR，特别是 #222 的实际 diff。若已被覆盖，改为验证差异并报告，不重复提交。
3. 只读参考仓库，按符号定位后验证磁盘内容；参考代码变化时重新比较 baseline.json。
4. 在单项白名单内先形成失败测试，再修复；若必须跨出范围，写明理由，不夹带其他问题。
5. 每项独立提交/PR。#169 与 #143 都涉及包校验但不捆绑；#166 与 #217 涉及清理但问题不同。
6. 本轮不构成任何真实 SAP 写入授权。后续实测须遵守会话和环境授权；不得连接生产、创建或释放传输、修改 E071/E071K，禁止直接数据库写入。
7. 不自动重放结果未知的写入/删除。失败后先报告已知状态和证据缺口。
8. 公共证据使用匿名对象/系统；参考仓库现有证据中的真实标识不得复制到上游 PR。代码移植保留 MIT 许可及原作者归属；内置 ADT 来源另见参考仓库 third-party/abap-adt-api/BASELINE.md 与 LICENSE。

## 最小验证与交付标准

Go 修复按影响运行定向测试，随后通常执行：

    go test ./pkg/adt ./internal/mcp ./cmd/vsp
    go build ./...
    go vet ./...
    git diff --check

Go 版本按 go.mod（当前 go 1.26）准备；不得为方便降级声明或删除失败测试。区分新增失败和主分支已有的平台/时序失败。单项的 wire 测试必须经过实际 HTTP/handler 路径；只测 helper 签名、字符串或复制实现结果不算修复证明。

本次参考实现验证已完成：

    npm test -- --runInBand --coverage=false --runTestsByPath src/__tests__/SessionSupervisor.test.ts src/__tests__/AdtSessionLifecycle.test.ts src/__tests__/DdicPrimitiveCreationAdapter.test.ts src/__tests__/DdicPropertyChangeWorkflow.test.ts src/__tests__/RepositoryObjectCleanupWorkflow.test.ts src/__tests__/ControlledSourceObjectAdt.test.ts src/__tests__/CdsSourceObjectCreationAdapter.test.ts src/__tests__/EnvironmentFile.test.ts src/__tests__/SessionResilienceConfig.test.ts

结果：9 suites / 86 tests 全通过，进程 exit 0。未运行全量测试、构建、Go 测试或真实 SAP。本次测试包含 mock，尤其 DTEL adapter 测试没有证明 Go wire parser 已正确。

每个修复最终报告：改动、失败前/修复后测试证据、真实 SAP 是否验证、遗留范围、PR closing/reference 策略。修复 #169 的一个子路径不得宣称关闭整个会话锁问题。

## 信息来源

- [上游分诊 PR #222](https://github.com/oisee/vibing-steampunk/pull/222)，本会话前轮实时获取；其报告是待核实线索，本文关键建议已再对照源码。
- [目标实时议程](D:/MyDev/SAP/vibing-steampunk/agenda/AGENDA.md:397)；其中也有过期条目，不能将所有未勾选项视为未实现。
- [参考项目入口](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/index.ts:206)、[参考会话监督器](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/lib/SessionSupervisor.ts:51)。
- Obsidian 相关关键词检索无命中；未读取或更改笔记。历史记忆只用于定位会话代码，当前结论均重新查阅源码。
