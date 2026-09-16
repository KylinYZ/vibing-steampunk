# #109 / PR #192：推进已有 DOMA、DTEL 创建贡献

状态：已有 PR 的后续交接，不新建重复 PR。建议继续现有 feat/ddic-domain-data-element 分支；若新 worktree，显式基于该 PR head，勿自动丢弃现有改动。

## 现状与证据边界

[issue #109](https://github.com/oisee/vibing-steampunk/issues/109) 请求 Domain/Data Element 创建。[PR #192](https://github.com/oisee/vibing-steampunk/pull/192) 是 KylinYZ 的既有实现，本会话前轮在线列表仍开放；main 的 creatable 类型还未包含 DOMA/DTEL。

本轮读过该 PR 的公开摘要和 #222 对它的分诊，但没有 checkout/逐行审查 #192 patch。以下是针对现有 PR 的复核清单，不是“已发现 PR 所有这些 bug”。开始时读取最新 diff/review/checks，再决定哪些真正需要修改。

## 参考仓库：有较完整实现

- [DdicPrimitiveCreationAdapter.ts:57](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/adapters/DdicPrimitiveCreationAdapter.ts:57)：校验目标不存在、包、属性、domain 引用、传输。
- execute:123：创建 shell → 身份复核 → LOCK → 属性 PUT → UNLOCK → activate → active 属性复读比较。
- [objectcontents.ts:98](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/api/objectcontents.ts:98)：Domain 类型/输出/值区间模型；Data Element getter/setter 在同文件。
- [objectcreator.ts:522](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/api/objectcreator.ts:522)：DDIC 创建目录/validation 映射。
- [RepositoryObjectCreationWorkflow.ts:92](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/safe/RepositoryObjectCreationWorkflow.ts:92)：未知结果阻止自动补偿。该文件存在工作区增量，见 baseline.json。
- [Domain 历史 DEV 证据](D:/MyDev/SAP/mcp-abap-abap-adt-api/docs/evidence/repository-creation-ddic-domain-real-dev-verified.md)、[Data Element 证据](D:/MyDev/SAP/mcp-abap-abap-adt-api/docs/evidence/repository-creation-data-element-real-dev-verified.md)。
- maturity manifest 的这两类记录为 create APPLIED、readback ACTIVE_VERIFIED、cleanup COMPLETED，且独立 absence 有证据。这里只确认历史记录内容，未重新在系统验证。

本轮 DdicPrimitiveCreationAdapter.test.ts 五个测试通过：域创建、非空默认值比较、元素创建、标志不匹配、引用域不存在。

## 移植重点

1. 沿用 PR 当前 API，不为套用 TS 而引入 preview plan/profile 全体系。
2. 复核 wire XML 与媒体类型，不能只创建 shell 就成功。属性写入处保持原 stateful 锁会话。
3. #153 若先合并，复用正确 DTEL parser；否则 #192 自己已有 parser 需比较，避免维护两个不同结构。
4. 验证 predefined 与 domain 引用，两者明确互斥/语义明确；保留四类 label 和长度限制。
5. Domain 固定值单值/区间、转换例程、输出属性的读回标准明确；SAP 省略默认与真实不同值必须区分。
6. 请求、激活结果、回读三层都检查；HTTP 200 不自动表示 active。
7. TS setter 的 finally 可能覆盖主错误、compensate 也不是通用模板，不直接复制。使用 Go #166 的错误处理原则；超时/断连不重放创建，不擅自删除无法证明属于此次的对象。
8. TypeScript 的固定环境元数据必须重新从目标包/系统解析；不能复制历史用户名、包、传输或 plan ID。

目标白名单以最新 #192 diff 为准，预计 pkg/adt/ddic_creation.go、crud.go、internal/mcp/handlers_crud.go、tools_register.go 与对应测试/说明。若借用 #153 的 reader，优先声明小依赖而非复制一份。

## 验收与推进顺序

- 先查看维护者是否要求缩小 scope、补 DCO/许可证、rebase 或补证据，不能猜“只差合并”。
- 为 create shell、属性、解锁、激活失败及回读不一致补真实 wire 的单元测试。
- 入口测试覆盖 properties_json 的实际 MCP 形态、DOMA/DD/DTEL/DE 名称路由。
- Go 自动化成功与 TS 历史实测分别报告；不能将 TS REAL_DEV_VERIFIED 标签移给 Go。
- 真实 DEV 生命周期另获授权后由专用对象验证；create/readback/transport/cleanup/absence 各自记录。数据库写入、创建/释放传输仍禁止。
- 继续原 PR，明确关联 #109；不要顺手加入所有 DDIC 对象或 DDLX。

## 新会话提示词

> 读取 D:/MyDev/SAP/vibing-steampunk/docs/pr-handoffs/2026-09-12/README.md 和 07-109-192-ddic-creation.md。先取得现有 PR #192 最新 diff、review 和 head，继续原贡献，不新开重复 PR。只读参考 D:/MyDev/SAP/mcp-abap-abap-adt-api 的 DDIC adapter、XML getter/setter 与历史证据，逐项核验而非假定 PR 有缺陷。补必要 wire/失败测试，保留结果未知不重放。没有单独授权不要做真实 SAP 或数据库写入。
