# #112：外部刷新 cookie 文件后恢复 MCP 读取

状态：方案交接，未实施。优先级 5。建议分支 codex/fix-112-cookie-file-reauth。

## 已确认差距

[issue #112](https://github.com/oisee/vibing-steampunk/issues/112) 的目标是：外部进程更新 --cookie-file 后，现有 MCP 无需重启恢复访问，不要求 MCP 自动登录浏览器。

Go [processCookieAuth](D:/MyDev/SAP/vibing-steampunk/cmd/vsp/main.go:754) 的普通 cookie 文件分支只加载一次；[buildClient](D:/MyDev/SAP/vibing-steampunk/cmd/vsp/cli.go:336) 也只传 WithCookies。已有 [LoadCookiesFromFile](D:/MyDev/SAP/vibing-steampunk/pkg/adt/cookies.go:14)、[callReauthFunc](D:/MyDev/SAP/vibing-steampunk/pkg/adt/http.go:921) 可复用。

## 参考仓库：无文件重载实现，有恢复约束

在参考 src 下搜索 cookieFile / COOKIE_FILE / LoadCookies / watchFile 等未找到同款配置链。其 AdtHTTP 管理内存 cookie，并从配置的凭据或 BearerFetcher 登录，不能把 reconnect 等同从磁盘重新加载 cookie。

可借鉴：
- [SessionSupervisor.ts:51](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/lib/SessionSupervisor.ts:51)：只读操作最多重放一次；修改类会话失败返回 REMOTE_RESULT_UNKNOWN。
- 同文件 reconnectIfNeeded 合并并发重连。
- [AdtHTTP.ts:261](D:/MyDev/SAP/mcp-abap-abap-adt-api/src/adt/AdtHTTP.ts:261) 在重连前清本地 cookie、CSRF/session 状态，保持配置会话模式。
- SessionSupervisor.test.ts 的只读重试一次、并发合并、修改不重放、显式登出不恢复，本轮通过。
- 不把早期只读 DEV 登录/登出验证写成“真实 cookie 文件过期恢复已验证”。

## 不能直接照分诊报告只加 callback

Go http.go:316 的重定向分支调用 reauth 后 retryRequest；同文件 session expiry、401 分支也会重试，未统一按业务操作分类。新加文件 callback 可能让更多请求进入恢复。锁句柄属于旧会话，换 cookie 不能让它在新会话继续可用。

因此必须同时证明恢复边界，不能只写 closure 就宣称解决。此次可限定普通 cookie 文件模式：自动恢复无锁窗口的 GET/HEAD；非读请求或已持锁场景明确返回会话失效/结果未知，不在新会话重放原写入。不要顺带重写全部 SSO/Basic Auth 重试策略；若需要范围扩展，先提出独立前置 PR。

## 实施方案

白名单：cmd/vsp/main.go、cli.go、pkg/adt/cookies.go、http.go/config.go 中必要的小配置接点及对应测试。

1. cookie-file 路径在启动时解析为确定路径；使用已有格式解析器，cookie-string 无重载源，保持原行为。
2. 文件模式的恢复回调重新读取、验证完整新文件；不存在/空/畸形/不可读要返回有界错误，不记录 cookie 内容。
3. 优先按失败触发重读，不先加轮询器/后台 watcher。复用现有互斥、超时、冷却机制，避免并发登录风暴。
4. 只在有效文件被接受后替换凭据；检查 cookie jar、CSRF、sessionID、cache 的一致性。同内容仍过期时终止，不循环。
5. 不把 cookie 发往任意新 origin，不跟随 IdP 重定向转发原凭据。
6. 明确“成功恢复”只恢复下一次可安全读；旧锁窗口已经失效不能续写。应检查 client 是否有 outstanding lock，或以显式请求/操作分类阻止恢复，不仅检查 HTTP 方法。
7. 浏览器外部写文件可能非原子；半写文件不得污染已加载状态，后续调用在文件完整后可再尝试。

## 验收

httptest + 临时 cookie 文件，无真实凭据：
- 首次 cookie A 被拒绝，磁盘换 B 后同一进程读恢复；新 cookie/CSRF 成对。
- 并发读仅有一次有效恢复；重复过期有限重试。
- 空、坏、缺失、半写文件给出明确错误。
- GET 发生于锁窗口、PUT/POST/DELETE 发生失效：不自动换会话继续执行旧操作，不增加写次数。
- cookie-string、Basic、已有 SSO 正常回归；显式退出不能被文件刷新偷偷撤销。
- cache 不把登录页保存为业务成功，也不把恢复前用户的数据交给恢复后的其他身份。

DEV/BTP 实测待另获授权，不能故意用真实用户错误密码诱发失败。

## 新会话提示词

> 读取 D:/MyDev/SAP/vibing-steampunk/docs/pr-handoffs/2026-09-12/README.md 与 05-112-cookie-file-refresh.md。复核 #112，在独立分支为普通 cookie-file 模式接入有界重载，先用假 HTTP 和临时文件证明无锁只读恢复及写入不重放。只读参考 TS SessionSupervisor；不能只加 ReauthFunc 就忽略 Go retryRequest 和旧锁窗口。必要的扩大范围先说明，不使用真实凭据，不执行 SAP/数据库写入。
