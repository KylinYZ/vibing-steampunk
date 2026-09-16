# #166：失败解锁、未知结果与准确清理状态

状态：方案交接，未实施。优先级 3。建议分支 codex/fix-166-failure-cleanup。

## 当前缺口

[issue #166](https://github.com/oisee/vibing-steampunk/issues/166) 不等于“所有锁都没释放”。Go 已有 [releaseLockAfterFailure](D:/MyDev/SAP/vibing-steampunk/pkg/adt/lock_release.go:29)，但仍有路径使用调用者已取消的 ctx，并丢弃解锁错误：

- [workflows_edit.go:404](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_edit.go:404)
- [workflows_deploy.go:142](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_deploy.go:142)、同文件 354
- [workflows_execute.go:206](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_execute.go:206)
- [workflows_source.go:670](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_source.go:670) 的创建 BDEF 路径，开工时重新确认。
- execute 的 defer [workflows_execute.go:184](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_execute.go:184)：忽略 DeleteObject 错误后设置 CleanedUp=true。

MCP lock_scope.go 另有在 caller ctx 上解锁且只写日志的路径。首 PR 可限定 ExecuteABAP 清理，再按同一规则分批接入；不要承诺一次关闭全部 #166。

[PR #217](https://github.com/oisee/vibing-steampunk/pull/217) 处理成功 DELETE 后代理 context 退休，不替代失败清理。开工先复核它的合并状态。

## 参考仓库：有状态语义，不是完整可抄补丁

- [AbapChangeWorkflow.ts:243](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/AbapChangeWorkflow.ts:243)：primaryError、rollback 状态与 unlockSucceeded 分别记录。
- [SourceObjectCreationAdapter.ts:188](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/adapters/SourceObjectCreationAdapter.ts:188)：保存 operationError，未知写入作为主结果，解锁失败阻止激活。
- [RepositoryObjectCleanupWorkflow.ts:313](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/RepositoryObjectCleanupWorkflow.ts:313)：DELETE 失败尝试解锁，记录失败，转为未知结果；不继续删除父对象。成功后 assertAbsent 才进入完成。
- [RepositoryObjectCreationWorkflow.ts:92](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/RepositoryObjectCreationWorkflow.ts:92)：未知创建结果不启动自动补偿。
- 本轮 RepositoryObjectCleanupWorkflow.test.ts 包含停止/单次删除/absence 测试并通过；该工作流及测试有既存未提交变更，见 baseline.json。

不能说“参考仓库已解决所有失败解锁”。例如 DdicPrimitiveCreationAdapter 的 finally 中解锁异常可能覆盖前面的异常，其 compensate 也不是通用解锁模板；只取已确认的良好模式。TS 没有 Go context.WithoutCancel 的同构代码，应复用 Go 自带 helper 的独立有限时清理上下文。

## 最小实施计划

第一块优先 ExecuteABAP：白名单 pkg/adt/workflows_execute.go、已有 lock_release.go（尽量不改）、新增 execute_cleanup_test.go 或已有测试。

1. 主操作取消/失败之后，以独立且有 deadline 的清理 ctx 处理已有锁；不以 context.Background 无限等待。
2. 原错误保留可识别身份，同时返回/记录解锁失败和人工处理提示，不用 unlock error 覆盖真正写入状态。
3. 删除只执行一次。确定删除成功后才报告清理成功；错误/超时不设 CleanedUp=true。
4. 若声明“对象不存在”，增加精确只读验证；成功 DELETE、absence verified 和 SAP 锁已释放是三个不同事实。
5. 删除结果未知不重试；任何“对象可能已删”的解锁结果也不能当作已证明不存在。
6. 不引入通用自动回滚，不删除非本次拥有的对象，不新增 SM12 自动解锁工具。

剩余 edit/deploy/BDEF/MCP 路径可另作小 PR；采用相同验收矩阵，保持既有主返回值兼容。

## 验收

- 主操作失败/取消后，清理请求仍可发出且有有限超时。
- PUT 失败且 UNLOCK 失败：二者均可见，主错误保留。
- DELETE 失败/超时：不报告 CleanedUp=true，不重试 DELETE。
- DELETE 成功但 search 仍存在：不宣称已验证缺失。
- 正常删除/KeepProgram 分支保持原义，拒绝策略不能通过清理绕过。
- 不存在执行主操作两次、释放其他用户锁或靠吞异常让测试通过的做法。

真实 DEV 锁残留验证只能另获授权后一次受控执行；取消 ctx 的主要证明应在 httptest 完成。

PR 使用 Refs #166；只有逐路径确认 issue 范围全部覆盖才考虑关闭。

## 新会话提示词

> 读取 D:/MyDev/SAP/vibing-steampunk/docs/pr-handoffs/2026-09-12/README.md 和 03-166-lock-cleanup.md。先复核 #217/#166 当前状态，在独立分支优先修复 ExecuteABAP 的失败清理及 CleanedUp 误报，复用 Go releaseLockAfterFailure；只读参考 TS 状态语义。测试 caller ctx 取消、双重失败、未知 DELETE 不重试。不要扩大到自动回滚/SM12 或批量重构；真实 SAP、数据库操作不在授权内。
