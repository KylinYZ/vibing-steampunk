# 暂缓方向与上轮判断修正

这些是上轮提过的较大方向，不属于前五项修复。此处只给边界和后续研究切口，不把局部关键词检索说成完整可行性审计。

## DAP / #184

[issue #184](https://github.com/oisee/vibing-steampunk/issues/184)。参考 TypeScript 有 ADT debugger 和受控 debug session，但在本次源码检索中未找到 DAP adapter 实现，不能作为现成 DAP 方案。

同时纠正一个容易沿用的旧结论：目标 Go main 已有 [cmd/vsp/debug_ui.go:39](D:/MyDev/SAP/vibing-steampunk/cmd/vsp/debug_ui.go:39)，注册 debug ui 并提供本地 HTTP UI。旧 issue/分诊所说“完全没有 UI”不能照抄进新 PR；已存在该文件不等于已实现通用 DAP。

后续独立研究：盘点当前 debug UI/RFC/ADT session 的实际共有边界，确认维护者要 DAP 还是增强现有 UI；只构建 attach/stackTrace/variables/step/disconnect 的最小 adapter 合同和 fake-debuggee conformance 测试，不先重写 debugger 或新增第二套 UI。

## OAuth2 / #99

[issue #99](https://github.com/oisee/vibing-steampunk/issues/99)。参考 AdtClient/AdtHTTP 有 BearerFetcher 和 bearer header，但本次未找到 BTP client_credentials / authorization-code token flow、服务密钥解析、刷新期限和 destination 集成的成套实现。只有 token 注入接口，不是完整 OAuth2 支持。

后续方案：先确认目标 endpoint 与授权模型，再接入有期限的 token provider、并发刷新、租户/系统隔离、脱敏、只读安全重试；显式区分应用身份与用户开发权限，不能假定 client credentials 可替代全部开发 ADT 登录。涉及官方协议时重新查一手资料和当前代码。

## 最小 ZADT_VSP 安装 / #138

[PR #138](https://github.com/oisee/vibing-steampunk/pull/138) 与 [agenda:26](D:/MyDev/SAP/vibing-steampunk/agenda/AGENDA.md:26) 已说明部分主干已落地，剩余 optional service/static reference 问题仍需真实安装验证。

参考 TypeScript 主要通过内置 ADT 客户端工作，无同款 ZADT_VSP APC 安装图；“不依赖 ZADT_VSP”只是另一种架构，不是可移植的安装器补丁。

后续方案：在目标仓库建立 service → ABAP class → 静态引用 → 所需接口/版本依赖矩阵；区分 src、embedded/abap 和实际 go:embed 的交付源。为可选 AMDP/abapGit 缺失定义可解释的 skip，并确保未部署类不被 APC handler 静态引用。先离线安装计划/生成器测试，真实安装单独授权。不要混入 #166、#112 或删除仍有用户依赖的 bridge 服务。

## 其他未列入本批的 issue

#161/#162 消息类、#123 revisions、#147 object structure、#151 embedded RFC 同步、#177 lint 等虽在前轮检索出现，但未进入前轮最终优先修复列表；本次不宣称完成它们的跨仓库审计。需要时按同样方式追加独立任务页。
