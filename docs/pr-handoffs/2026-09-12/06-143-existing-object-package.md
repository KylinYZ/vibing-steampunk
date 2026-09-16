# #143：已有对象更新缺省 package 的正确解析

状态：方案交接，未实施。优先级 6。建议分支 codex/fix-143-update-package-resolution。

## 问题范围

[issue #143](https://github.com/oisee/vibing-steampunk/issues/143) 的当前残余在 [WriteSource:238](D:/MyDev/SAP/vibing-steampunk/pkg/adt/workflows_source.go:238)：顶层 checkMutation 只收到 opts.Package/Transport，没有 ObjectURL。已有对象更新省略 package 且配置 AllowedPackages 时，在存在性探测/分派之前就报错。

这与 #169 不同：#143 是还没有锁就拒绝合法更新；#169 是取锁后多发无状态查询破坏会话。EditSourceWithOptions 有 objectURL，不应被概括为同样缺陷。

## 参考仓库：有元数据解析模式，无同款 allowlist 补丁

- [AbapObjectResolver.ts:10](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/AbapObjectResolver.ts:10) 通过类型/名字搜索唯一对象，读取结构并确定 object/source/lock URL，提取 packageName。
- [AbapChangeWorkflow.ts:103](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/AbapChangeWorkflow.ts:103) preview 接受对象身份与 transport，不要求调用者再提供 package。
- [validateTransport:351](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/AbapChangeWorkflow.ts:351) 依据 transportInfo.DEVCLASS 或已解析包验证。

限制：参考 resolver 只支持 PROGRAM/INCLUDE/CLASS/FUNCTION_MODULE，不覆盖 Go 所有类型；参考包策略主要限制传输包与角色/命名空间，不能当作 Go AllowedPackages 的现成授权规则。

## 最小方案

白名单：pkg/adt/workflows_source.go、mutation_gate.go/marker 仅必要时、对应测试。

1. 保留顶层纯本地操作/传输策略校验；不要为了跳过包查询而取消整个 gate。
2. update 必须使用真实对象 URL 解析并校验归属；调用者显式 package 不能伪装对象实际包。
3. upsert 先判定对象存在，再选择已存在更新校验或新建显式 package 校验；不能凭 package 为空默认创建到 $TMP。
4. 查找失败不等于“不存在”：403、timeout、5xx 不得降级为 create；只有确定不存在才进入 create。
5. 审计每个 delegated update：PROG、CLAS（含 include/method）、INTF、INCL、DDLS、BDEF、SRVD、TABL、SRVB 当前支持分支。只把已经落实的类型列为修复，不能假定所有分支都 gateAndMark。
6. 包校验必须在 LOCK 前；复用逐对象 marker，避免修 #143 又引入 #169。
7. 与 #169 不要求互相依赖，若更改同一 helper，顺序合并并重新运行相邻测试。

## 验收

- 允许包已有对象，update/upsert 不传 package 正常进入更新。
- 禁止包已有对象仍拒绝；伪造允许 package 不能授权实际禁止包。
- 新对象缺 package、禁止包创建按原契约拒绝。
- 对象查找 403/超时不会调用 CreateObject。
- 顶层禁写/传输策略仍拒绝且没有写请求。
- 针对声称支持的每个对象分支有 gate 路径证据；MCP/CLI 至少各一个真实入口测试。
- HTTP 顺序确认查包在 LOCK 前。

PR 可只先覆盖明确类型并使用 Refs #143。全部 issue 范围与入口通过后再 Closes；不捆绑 #169 或改为默认放宽包白名单。

## 新会话提示词

> 在 D:/MyDev/SAP/vibing-steampunk 读取 docs/pr-handoffs/2026-09-12/README.md 和 06-143-existing-object-package.md。独立分支修复 WriteSource 已有对象更新省略 package 的 gate 顺序；从实际元数据校验包，create/upsert 分开，不把查找失败当不存在。只读借鉴 TS resolver，保留全部策略，验证不会重新引入 #169。只做本地代码/测试，不执行真实 SAP/数据库写入。
