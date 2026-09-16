# #169：自动加锁写入前完成包校验

状态：方案交接，未实施。优先级 1。建议分支 codex/fix-169-self-lock-package-check。目标只在 vibing-steampunk；参考项目只读。

## 问题与最小边界

[issue #169](https://github.com/oisee/vibing-steampunk/issues/169) 是跨工具锁句柄问题的跟踪项。#183 已让若干工具支持省略 lock_handle，但省略句柄并不代表所有会话问题消失。

当前确定的静态链路：

    handleUpdateSource
      -> withObjectLock
      -> LockObject
      -> UpdateSource
      -> checkMutation
      -> checkMutationPackage / SearchObject（无状态）
      -> PUT（原锁句柄可能失效，423）

证据：[handlers_crud.go:107](D:/MyDev/SAP/vibing-steampunk/internal/mcp/handlers_crud.go:107)、[lock_scope.go:24](D:/MyDev/SAP/vibing-steampunk/internal/mcp/lock_scope.go:24)、[crud.go:187](D:/MyDev/SAP/vibing-steampunk/pkg/adt/crud.go:187)、[mutation_gate.go:79](D:/MyDev/SAP/vibing-steampunk/pkg/adt/mutation_gate.go:79)。本次未在 SAP 重现 423；以上为当前代码结构与已有缺陷语义的交叉确认。

首个 PR 只解决 MCP UpdateSource 的自动加锁路径。不同时改手工句柄协议、CreateTestInclude、UpdateClassInclude、DeleteObject 或全局会话默认值。相邻调用者列为后续审计，不暗示已修。

## 参考仓库是否有解决方案

有设计模式，没有同构补丁：

- [src/index.ts:206](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/index.ts:206)：主客户端设置为 stateful。
- [AdtClient.ts:500](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/AdtClient.ts:500)：可选 statelessClone 使用另一个 HTTP 客户端，且不拥有 keepalive。
- [AbapChangeWorkflow.ts:157](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/AbapChangeWorkflow.ts:157)：重验证在锁之前，锁、写入、解锁在同一次 apply 内；激活在解锁之后。
- [SessionSupervisor.ts:51](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/lib/SessionSupervisor.ts:51)：写操作会话失败不重放。

这些不能证明 Go 只要全局 stateful 就会好，也不能证明参考仓库的全部 legacy raw tools 已防止跨调用干扰。参考侧命名空间/角色策略不等同 Go 包白名单。

## 推荐 Go 修复

优先复用已经存在的 [PrepareSourceUpdate](D:/MyDev/SAP/vibing-steampunk/pkg/adt/mutation_gate_marker.go:89)，[handlers_deploy.go:235](D:/MyDev/SAP/vibing-steampunk/internal/mcp/handlers_deploy.go:235) 已有同类使用。

在自动加锁分支，先用规范对象 URL 和本次 transport 调用 PrepareSourceUpdate；成功后把返回的 context 同时传给加锁和闭包里的 UpdateSource。该 marker 只跳过同一对象已完成的网络包查询，不跳过操作和传输策略。

不要直接调用 TypeScript 私有方法，也不要将 Go 未导出 marker 当成 internal/mcp 可调用接口。不要在用户已提供句柄后补发无状态包查询——那个锁窗口已经开始，必须另行设计兼容语义。

白名单：internal/mcp/handlers_crud.go、internal/mcp/lock_scope_test.go 或新的同目录行为测试。只有证明确需调整 helper 时才修改 lock_scope.go；通常无需改全局 HTTP 或策略实现。

## 必须通过的验收

- 开启 AllowedPackages，省略 lock_handle，对允许的已有对象：包查询在 LOCK 之前，LOCK 至 PUT/UNLOCK 间无 stateless 请求，PUT 仅一次。
- 非允许包、只读、禁止操作、传输策略拒绝：在 LOCK/PUT 前拒绝。
- A 对象验证结果不能授权 B 对象；对象 URL 与 /source/main 的规范化覆盖命名空间。
- 不开启白名单：保持原有成功行为。
- 手工 lock_handle 路径行为明确且无新增查包请求；不声称已修其根本问题。
- 请求失败、取消不触发写入重放；遗留解锁问题按 #166 单独跟踪。

参考测试模式：[session_affinity_test.go:104](D:/MyDev/SAP/vibing-steampunk/pkg/adt/session_affinity_test.go:104)、同文件 TestPrepareSourceUpdate_MarksTheObjectItChecked。当前 lock_scope_test.go 主要验证 schema 和签名，缺少此 HTTP 顺序测试。

实测若另获授权：仅指定 DEV 的一次受控已有测试对象更新，读取前后内容及锁状态；对象清理单列。不要为了测试反复制造遗留锁。

## PR 与续跑

建议标题：fix(mcp): validate package access before self-locking source updates。
使用 Refs #169 / Refs #181，不使用 Closes #169；报告只覆盖自动加锁 UpdateSource。

新会话提示词：

> 在 D:/MyDev/SAP/vibing-steampunk 读取 docs/pr-handoffs/2026-09-12/README.md 和 01-169-self-lock-session.md，先复核上游 issue/PR 是否已覆盖。按单项范围在独立 worktree/分支修复 MCP UpdateSource 自动加锁前包校验；只读参考 D:/MyDev/SAP/mcp-abap-abap-adt-api。补 HTTP 请求顺序和拒绝路径测试，不改手工句柄协议，不放宽策略，不执行真实 SAP 或数据库写入。完成定向测试、构建和 diff 检查，报告尚未覆盖的 #169 路径。
