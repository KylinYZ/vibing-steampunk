# #153：修复 DTEL 信息读取的内容协商和解析

状态：方案交接，未实施。优先级 2。建议分支 codex/fix-153-data-element-info。

## 确认的问题

[issue #153](https://github.com/oisee/vibing-steampunk/issues/153) 对应 [GetTypeInfo](D:/MyDev/SAP/vibing-steampunk/pkg/adt/client.go:1417)：发送 application/xml，解析器又假定 type/length/decimals 是根节点属性。只修 Accept 很可能把“406”变成“成功返回空字段/0”。

同仓库 [GetDataElementLabels](D:/MyDev/SAP/vibing-steampunk/pkg/adt/i18n.go:95) 已对相同端点使用 application/vnd.sap.adt.dataelements.v2+xml，但只读取标签，不等于 TypeInfo 已完成。

## 参考仓库：有可移植的字段解析

- [objectcontents.ts:478](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/api/objectcontents.ts:478)：getDataElementProperties，读取 active/inactive/workingArea 文档。
- 根文档属性为 adtcore:name、description、type 等，包来自 adtcore:packageRef。
- 类型来自内层 dtel:dataElement：typeName、dataType、dataTypeLength、dataTypeDecimals；另有四组 field labels。
- setter [objectcontents.ts:407](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/api/objectcontents.ts:407) 明确外层 blue:wbobj 和内层 dtel:dataElement，typeKind 为 domain 或 predefinedAbapType。
- [DdicHandlers.ts:117](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/handlers/DdicHandlers.ts:117) 提供真实读取入口。
- 历史 [Data Element DEV 证据](D:/MyDev/SAP/mcp-abap-abap-adt-api/docs/evidence/repository-creation-data-element-real-dev-verified.md) 有 active 回读；不是本轮实测。

关键差异：TypeScript getter 未指定该 versioned Accept，它沿用 [AdtHTTP.ts:220](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/AdtHTTP.ts:220) 的 */*。因此 Go 的 Accept 修复参考 Go i18n 已有做法；字段结构参考 TypeScript，不要错误写成“TS 已使用同样 v2 头”。

TS parser 当前不显式返回 typeKind，且数值缺失/无效会折叠为 0；不能将这些宽松行为机械复制到 Go。

## Go 实施建议

白名单：pkg/adt/client.go；必要时提取 pkg/adt/data_element.go 和对应测试；internal/mcp/handlers_read.go 仅在返回字段扩展确需时修改。

1. 保留 GetTypeInfo 名称与已有字段兼容性，使用已有验证过的 versioned Accept。
2. 建立 wire DTO，按 XML namespace/local name 解析 wbobj/dataElement；支持前缀变化，XML 转义正确。
3. Name/Description 来自元数据；Type 保持对调用者表示底层 dataType 的既有意图；Length/Decimals 读取对应内层值。增补 Domain/TypeKind 时说明语义及 JSON 兼容性。
4. domain 引用型与 predefined 类型分开建 fixture。若 domain 型响应不含底层长度，明确未知或执行限定的 domain 只读补查；不得把 domain 名称直接当底层类型。
5. 区分对象不存在、内容不受支持、XML 异常和有效零值；HTML 或缺少必需结构不得返回“成功且全空”。
6. 名称大小写、带 /namespace/ 的 PathEscape 沿用已有安全 URL 模式。
7. 不在这个 PR 扩展所有 GetSource 对象类型；#109 创建独立处理。

## 验收与证据

- HTTP stub 验证资源路径、Accept、XML 解析后的实际 Name/Type/Length/Decimals/Domain，而非只判断无 error。
- 固定类型 CHAR、带小数类型、domain 引用、不同前缀、404、406、畸形 XML、HTML 登录页。
- MCP handleGetTypeInfo 经真实 client 得到非空信息；不要求修改 profile/新增工具。
- 现有 GetDataElementLabels 不回归。
- 若获只读 DEV 授权，对一个 domain 型和一个 predefined DTEL 与 ADT 原文对照，记录匿名 shape；不创建对象验证读路径。

参考 adapter 的测试通过只证明模拟属性生命周期；本轮未找到 getter 原始 XML 解析的独立测试，Go 必须补自己的 wire fixture。

建议标题：fix(adt): decode data element type information using the ADT document contract。
可在问题完整覆盖后 Closes #153；若未满足 issue 要求字段则 Refs。

## 新会话提示词

> 在 D:/MyDev/SAP/vibing-steampunk 读取 docs/pr-handoffs/2026-09-12/README.md 和 02-153-data-element-read.md，复核 #153 和在途 PR。独立分支修复 GetTypeInfo 的 Accept 与实际 DTEL XML 解析，保持已有调用契约；只读参考 TypeScript objectcontents.ts，不只改请求头，不混入 DDIC 创建。补 wire 和 MCP 输出测试、构建与 diff 检查。未经单独授权不连接真实 SAP，不执行数据库写入。
