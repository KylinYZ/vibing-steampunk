# #117：CLI 安全参数从解析到客户端的完整传递

状态：方案交接，未实施。优先级 4。建议分支 codex/fix-117-cli-transport-opt-in。

## 当前代码与原因

[issue #117](https://github.com/oisee/vibing-steampunk/issues/117) 报告 --allow-transportable-edits 在子命令 unknown flag，纯环境变量方式也未生效。

- [main.go:146](D:/MyDev/SAP/vibing-steampunk/cmd/vsp/main.go:146) 注册在 rootCmd.Flags()，不是 PersistentFlags()。
- [cli.go:75](D:/MyDev/SAP/vibing-steampunk/cmd/vsp/cli.go:75) 的 resolveSystemParams 与 MCP resolveConfig 是两条配置链。
- 命名系统分支 cli.go:137 能合并部分环境参数；纯环境分支 cli.go:166 未填写 AllowTransportableEdits 等传输安全字段。
- buildClient 在 cli.go:248 已有消费部分 systemParams 的逻辑。因此“flag 能解析”不代表其值到达实际客户端。

## 参考仓库：没有该问题的直接实现

TypeScript MCP 无 Cobra CLI 子命令结构，也没有同款 SAP_ALLOW_TRANSPORTABLE_EDITS 规则。不能将 profile 白名单、DEV/PRD role、命名空间策略等同于 Go transport opt-in。

可借鉴的仅是：
- [SessionResilienceConfig.ts:14](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/config/SessionResilienceConfig.ts:14) 显式布尔解析，识别 false 与缺省。
- [RuntimeGuardrails.ts:41](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/config/RuntimeGuardrails.ts:41) 统一配置构建及参数范围校验。
- [ToolOperationPolicy.ts](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/config/ToolOperationPolicy.ts) 区分配置可见与实际执行边界。
- EnvironmentFile.test.ts、SessionResilienceConfig.test.ts 本轮通过；与 Cobra 无直接覆盖关系。

## 方案与范围

优先只修 AllowTransportableEdits 及其必需传递链，不在首 PR 改所有安全选项、MCP -s 行为或认证模式。

白名单：cmd/vsp/main.go、cli.go、cli_safety_test.go 或新的配置集成测试；确需复用既有配置类型时再扩展，不能降低 adt.Safety 默认值。

1. 让明确支持的 flag 对目标子命令可见；检查局部同名 flag 与 viper.BindPFlag 的绑定位置。
2. 从 Cobra 的 Changed 状态读取显式值并映射到 systemParams，确保 false 不被当成“未传”。
3. 纯环境分支补全该 opt-in；命名系统、缺省系统、纯 SAP_* 环境三种模式各自覆盖。
4. 写清优先级。建议采用已声明的显式 flag > 对应环境值 > 系统配置 > 默认，但当前命名系统有 config || env 语义；必须复核现有文档/测试，避免无意改变整套策略。严格部署不允许被 flag 放宽的设置应单列，而非隐式混用。
5. 默认仍不允许 transportable edits。单独开启该选项不能绕过 ReadOnly、包限制或其他限制。
6. 对其余 --allowed-packages/ops/transports/enable-transports/transport-read-only，记录逐项现状；不凭一个参数修复就宣布全部 CLI 安全选项修复。

## 验收

从真正 Cobra 命令解析入口进入 resolver/buildClient，以本地假 ADT server 测试：
- 子命令接受显式 true/false；未传使用原默认。
- 命名系统、默认系统、纯环境三种来源分别成功传递。
- 环境 true 配合显式 false 的结果与约定优先级一致。
- 非法值有明确诊断；不悄悄按 false。
- 未 opt-in 的写入在网络写之前拒绝；opt-in 后仍不能突破只读/包限制。
- MCP root 启动及其他不相关参数不回归。

单纯运行 --help 或测试手工 new systemParams 不是验收，必须穿过真实配置解析链。可先只关闭 issue 的 flag/env 两个明确问题；其余参数列后续。

## 新会话提示词

> 在 D:/MyDev/SAP/vibing-steampunk 读取 docs/pr-handoffs/2026-09-12/README.md 和 04-117-cli-safety-config.md，复核 #117。独立分支贯通 --allow-transportable-edits 的 Cobra、系统配置/环境与客户端策略，不只改 flag 注册；明确 false 和优先级，覆盖三种配置模式的入口测试。参考 TS 只用于解析原则，不移植 profile 架构，不顺带重构所有配置，不执行真实 SAP/数据库写入。
