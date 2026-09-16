# ZVSP_COMPAT — SAP 7.51 仓库操作兼容门面

`ZVSP_COMPAT_751` 是函数组 `ZVSP_COMPAT` 中唯一的函数模块，用于不提供
表创建与文本元素 ADT 资源的老系统（ADT 的这两类资源自 7.52 SP00 才有，
7.51 上 VSP 的 `CreateTable` / `WriteTextPool` 会收到 404）。

它刻意只暴露一个封闭的小操作集，而不是做成通用函数模块代理：

| `I_OP` | 标准 API | 输入 | 结果 |
|---|---|---|---|
| `TEXTPOOL_GET` | `READ TEXTPOOL` | `I_PROGRAM`、`I_LANGUAGE` | 完整 `E_TEXTPOOL` |
| `TEXTPOOL_SET` | `RPY_TEXTELEMENTS_INSERT` | 完整 `I_TEXTPOOL`、包、语言、传输号 | 写入并读回核验（`E_TEXTPOOL`） |
| `TABLE_CREATE` | `RPY_TABLE_INSERT` + `DDIF_TABL_ACTIVATE` | 标量表头参数 + `I_FIELDS_JSON` | 创建并激活的透明表 |

VSP（Go 端）在 ADT 返回 404 时自动改走本门面：`pkg/adt` 的
`CompatFallback` 接口在 `internal/mcp` 里被接到经典 RFC 连接上，
`CreateTable` 与文本池读写无需换工具、无需换参数。

## 部署

### 方式一（推荐）：VSP 一条命令自动部署

ADT 模型里函数模块的接口就定义在源码签名中，因此 VSP 的
`CreateObject` 工作流可以完整创建（创建 → 置 RFC 标志 → 写签名源码 → 激活），
不需要手工 SE37：

1. 传输请求（没有可用的就先建一个）：

   ```jsonc
   SAP(action="system", params={"type":"create_transport", "description":"ZVSP_COMPAT facade", "package":"<开发包>"})
   ```

2. 建函数组：

   ```jsonc
   CreateObject(objectType="FUGR", name="ZVSP_COMPAT", description="VSP 7.51 compatibility facade",
                package_name="<开发包>", transport="<请求号>")
   ```

3. 建函数模块（`rfc_enabled` 会经 SetFunctionModuleProcessingType 单独置位；
   `source` 的签名就是接口定义）：

   ```jsonc
   CreateObject(objectType="FUGR/FF", name="ZVSP_COMPAT_751", parent_name="ZVSP_COMPAT",
                rfc_enabled=true, package_name="<开发包>", transport="<请求号>",
                source=<zvsp_compat.fugr.zvsp_compat_751.func.abap 的完整内容>)
   ```

4. 验证（接口参数应与下表一致）：

   ```jsonc
   SAP(action="rfc", target="ZVSP_COMPAT_751", params={"op":"describe"})
   ```

`DeployFromFile` 也能部署本文件（文件名已符合 `{fugr}.fugr.{func}.func.abap`
约定），但那条通用路径不设置 RFC-enabled 标志——用它就要事后在 SE37 勾一次
"远程可调用的模块"，所以推荐上面的 `CreateObject` + `rfc_enabled` 组合。

### 方式二（备选）：手工 SE37

全部参数为标量或两个全系统稳定的标准结构（`TEXTPOOL`：`ID/KEY/ENTRY`），
手工逐个建立即可：

| 方向 | 参数 | 类型 | 说明 |
|---|---|---|---|
| Import | `I_OP` | `CHAR20` | `TEXTPOOL_GET` / `TEXTPOOL_SET` / `TABLE_CREATE` |
| Import | `I_PROGRAM` | `PROGRAMM` | 文本池操作的目标程序 |
| Import | `I_LANGUAGE` | `SYLANGU` | 缺省 `SY-LANGU` |
| Import | `I_DEVCLASS` | `DEVCLASS` | 开发包（SET 必填） |
| Import | `I_TRANSPORT` | `TRKORR` | 传输请求（可传输包必填） |
| Import | `I_TABLE` | `RPY_TABL-TABLNAME` | 表名 |
| Import | `I_DESCRIPTION` | `DDTEXT` | 表描述（CHAR60） |
| Import | `I_DELIVERY_CLASS` | `CHAR1` | 交付类，空 = `A` |
| Import | `I_TABLE_CATEGORY` | `CHAR8` | 表类别，空 = `TRANSP` |
| Import | `I_TABART` | `CHAR5` | 数据类（TABART），空 = `APPL0` |
| Import | `I_BUFFERING` | `CHAR1` | 缓冲类型，空 = 不缓冲 |
| Import | `I_FIELDS_JSON` | `STRING` | 字段定义 JSON（仅 `TABLE_CREATE`） |
| Export | `E_RC` | `I` | 0 = 成功；8 = 参数/权限失败；12 = 建表成功但激活失败 |
| Export | `E_ACTIVATION_RC` | `I` | `DDIF_TABL_ACTIVATE` 的返回码 |
| Export | `E_MESSAGE` | `STRING` | 失败原因或读回摘要 |
| Tables | `I_TEXTPOOL` | `TEXTPOOL` | 完整文本池（SET 的输入） |
| Tables | `E_TEXTPOOL` | `TEXTPOOL` | 完整文本池（GET 的输出、SET 的读回） |

手工部署时，建完参数后把处理类型改为 **远程可调用的模块**
（Remote-Enabled Module），然后激活。全部逻辑内联在函数体内：函数组里
不需要 FORM 例程或全局数据。
VSP 部署源文件为
[`zvsp_compat.fugr.zvsp_compat_751.func.abap`](zvsp_compat.fugr.zvsp_compat_751.func.abap)。

## 字段 JSON 协议（`I_FIELDS_JSON`）

```json
[
  {"name": "MANDT",  "type": "CLNT",  "len": 3, "key": true,  "description": "Client"},
  {"name": "CARRID", "type": "CHAR",  "len": 3, "key": true,  "description": "Carrier"},
  {"name": "CARRNAME", "dataElement": "S_CARRNAME", "notNull": true},
  {"name": "CURRCODE", "type": "CUKY", "description": "Currency"}
]
```

- `type` 接受 DDIC 内置类型码；`DATE`/`TIME`/`CLIENT`/`MANDT` 会被归一化，
  `STRING`/`RAWSTRING` 在 ABAP 端映射为内部码 `STRG`/`RSTR`。
- `dataElement` 与 `type` 二选一，给出 `dataElement` 时优先（走 `ROLLNAME`）。
- 客户端字段由调用方放入数组，函数不自动添加——首字段若已是 CLNT 类型
  （VSP 会传 `MANDT`），不会重复。
- 字段名超 30 字符、缺 `name`、缺类型会被 `E_RC=8` 拒绝。

表头缺省与 Go 端 `CreateTableOptions` 一致：交付类 `A`、类别 `TRANSP`、
数据类 `APPL0`、不缓冲。任何一端直接调用也都成立。

## 安全模型

- 只接受 `Z`/`Y` 开头的对象名；不提供任意函数调用通道。
- 非本地包（`$` 开头之外）必须带 `I_TRANSPORT`，仓库归属由标准 API 登记。
- `TEXTPOOL_SET` 收到的是**完整** `TEXTPOOL` 表，不是补丁：调用方必须先
  `TEXTPOOL_GET` 读回当前池、合并增删改后再整表写入。
- `TABLE_CREATE` 的激活检查（`AUTH_CHK = 'X'`）永不关闭。
- 只授予目标 SAP 用户所需的开发/CTS 权限，不要给技术 RFC 用户
  整库的仓库修改权限。

## 失败约定

调用方把非零 `E_RC`、或 `E_ACTIVATION_RC > 4` 视为失败。

- `TEXTPOOL_SET` 失败不改变任何条目；成功后 `E_TEXTPOOL` 是读回的活动版本，
  `E_MESSAGE` 带条数摘要。
- `TABLE_CREATE` 的 `E_RC=12` 表示**表已创建但激活失败**：会留下一个未激活
  的 DDIC 定义，需要按 SE11 激活日志人工处理；本函数不自动删除。

## 部署限制

受控创建器（preview/apply 计划体系）在实测的 7.51 系统上不可用：其对象
存在性校验依赖 ADT 搜索资源，而该系统（Basis 816）对所有搜索查询一律
回答 400——两套受控创建流程都卡在 resolve 阶段。`CreateObject` /
`DeployFromFile` 工作流用 GET 对象 URL 判断存在性，不依赖搜索，因此
方式一是唯一在本系统验证可行的自动部署路径。
