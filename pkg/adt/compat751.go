package adt

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
)

// --- 7.51 兼容通道（CompatFallback） -----------------------------------------
//
// SAP NetWeaver 7.52 SP00 才开始提供 ADT 的"数据库表创建/源码编辑"与
// "文本元素"资源。在此之前的老系统（7.51 及更早）上，CreateTable 与
// 文本池读写会收到 404——那不是权限或网络问题，而是后端根本没有该资源。
//
// 兼容通道是一个部署在目标系统上的受控 RFC 门面（abap/src/zvsp_compat，
// 函数模块 ZVSP_COMPAT_751）：只暴露三个封闭动作（表创建、文本池读写），
// 只接受 Z/Y 对象，不是通用函数调用代理。pkg/adt 只定义本接口；RFC 实现
// 在 pkg/saprfc（open-rfc-go 客户端），由 internal/mcp 在启动时注入。
// 没有 RFC 连接或门面未部署时，兼容通道缺席，行为与从前一致（404 原样报错）。

// CompatFallback 是老版本系统上缺失 ADT 资源时的受控 RFC 通道。
// 实现方在 ABAP 端负责仓库归属（包/传输请求）、授权检查与激活结果。
type CompatFallback interface {
	// TableCreate 创建并激活一张透明表。实现方收到的是已通过
	// 变更门（checkMutation）的请求；返回非 nil 错误即失败。
	TableCreate(ctx context.Context, req CompatTableCreate) error

	// TextPoolGet 读回一个程序在指定语言的完整文本池。池里可能含有
	// 本通道不维护的条目（例如经典 H 标题），原样返回——调用方在全池
	// 覆盖写回时必须保留它们，漏带等于删除。
	TextPoolGet(ctx context.Context, program, lang string) ([]TextPoolEntry, error)

	// TextPoolSet 以完整 TEXTPOOL 表覆盖一个程序的文本池。
	// Entries 必须是合并后的完整池——这是覆盖写，不是补丁。
	// 返回写入后读回的池，供调用方核验。
	TextPoolSet(ctx context.Context, req CompatTextPoolSet) ([]TextPoolEntry, error)
}

// CompatTableCreate 是 TableCreate 的请求。字段与 CreateTableOptions 对齐，
// DataClass/Buffering 留空时由 ABAP 端取 SE11 缺省（APPL0、不缓冲）。
type CompatTableCreate struct {
	Name          string       // 表名（已大写、已过 1-30 字符校验）
	Description   string       // 表描述
	Package       string       // 开发包
	Transport     string       // 传输请求（可传输包必填）
	DeliveryClass string       // 交付类（A/C/L/S/G/E/W），空 = ABAP 端补 A
	TableCategory string       // 表类别，空 = TRANSP
	DataClass     string       // 技术设置 TABART（数据类），空 = APPL0
	Buffering     string       // 缓冲类型，空 = 不缓冲
	Fields        []TableField // 字段定义（含客户端字段）
}

// CompatTextPoolSet 是 TextPoolSet 的请求。
type CompatTextPoolSet struct {
	Program   string          // 目标程序名（Z/Y）
	DevClass  string          // 开发包（ABAP 端 RPY_TEXTELEMENTS_INSERT 必填）
	Transport string          // 传输请求（可传输包必填）
	Language  string          // 语言
	Entries   []TextPoolEntry // 合并后的完整文本池
}

// compatTextRoute 记录本系统对 textelements ADT 资源的可用性。
// 读路径的 404 会被 readTextDocument 吞成"空文档"，因此缺资源的系统上
// 读取会静默返回空池、写入会在加锁一步失败——必须在入口按路由分流，
// 且结论按 client 记忆，避免每次调用都付一次探测往返。
type compatTextRoute struct {
	rfc atomic.Pointer[bool] // nil = 未探测；true = 该系统缺资源，走 RFC 门面
}

// SetCompatFallback 把受控 RFC 门面挂到客户端上。只在启动路径写一次，
// 之后各请求只读，因此不加锁。
func (c *Client) SetCompatFallback(f CompatFallback) { c.compatFallback = f }

// --- 表创建的兼容触发 ---------------------------------------------------------

// createTableViaCompat 把 CreateTable 的 ADT 流程翻译成受控 RFC 门面调用。
// 只在 ADT 建表资源返回 404（该版本根本不提供）时进入；安全门
// （checkMutation）已由 CreateTable 在分叉之前跑过，两个通道等价受控。
// adtErr 是触发分叉的原始 404——兼容通道再失败时两者一起返回，
// 调用方一次就能看到完整因果。
func (c *Client) createTableViaCompat(ctx context.Context, opts CreateTableOptions, adtErr error) error {
	if c.compatFallback == nil {
		return fmt.Errorf("creating table object: this SAP release does not offer the ADT table resource (7.52+), and no RFC compat facade (ZVSP_COMPAT_751) is configured (%v)", adtErr)
	}

	// 客户端字段是 SE11 建表语义的一部分，兼容通道在 Go 端补齐，
	// ABAP 端不再自动添加。CreateTableOptions 没有数据类/缓冲选项，
	// 门面按 ABAP 端缺省（APPL0、不缓冲）处理，与 ADT DDL 路径一致。
	req := CompatTableCreate{
		Name:          opts.Name,
		Description:   opts.Description,
		Package:       opts.Package,
		Transport:     opts.Transport,
		DeliveryClass: opts.DeliveryClass,
		TableCategory: opts.TableCategory,
		Fields:        prependClientField(opts.Fields),
	}
	if err := c.compatFallback.TableCreate(ctx, req); err != nil {
		return fmt.Errorf("creating table %s: the ADT table resource is not available on this release (%v), and the 7.51 RFC compat facade failed: %w", opts.Name, adtErr, err)
	}
	return nil
}

// prependClientField 在字段列表最前补一个标准客户端字段（MANDT, CLNT(3),
// 主键, NOT NULL），与 generateTableDDL 自动生成 `key client` 的行为对应。
// 调用方自己已经给了 CLNT 类型的客户端字段时不重复加——否则建表会得到
// 两个键位相同的客户端字段，被 RPY_TABLE_INSERT 拒绝。
func prependClientField(fields []TableField) []TableField {
	for _, f := range fields {
		switch strings.ToUpper(strings.TrimSpace(f.Type)) {
		case "CLNT", "MANDT", "CLIENT":
			return fields
		}
	}
	out := make([]TableField, 0, len(fields)+1)
	out = append(out, TableField{Name: "MANDT", Type: "CLNT", Length: 3, IsKey: true, NotNull: true, Description: "Client"})
	return append(out, fields...)
}

// --- 文本池的兼容路由 ---------------------------------------------------------

// textPoolNeedsCompat 报告 t 所在系统是否要走兼容通道。
//
// 探测语义：对象级 textelements 资源（不带 /source/<kind>）在 7.52+ 的系统
// 上对存在的程序返回 200；在 7.51 上整个资源集合都不存在，返回 404。
// 404 也可能是对象本身不存在——那是另一个错误，必须区分开：
//
//	对象级资源 404 + 宿主对象 200 → 版本缺资源 → true
//	对象级资源 404 + 宿主对象 404 → 对象不存在 → 报错，不路由
//	其余                          → 原 ADT 路径
//
// 类（CLAS）文本符号没有经典 READ TEXTPOOL 等价物，兼容通道不支持；
// 缺资源的系统上读到空池是误导，这里显式拒绝。
func (c *Client) textPoolNeedsCompat(ctx context.Context, t TextPoolTarget) (bool, error) {
	if v := c.compatText.rfc.Load(); v != nil {
		return *v, nil
	}

	resp, err := c.transport.Request(ctx, t.resource(), &RequestOptions{
		Method: http.MethodGet,
		Accept: "*/*",
	})
	// 资源正常应答 → ADT 路径；只有"对象级 404 + 宿主存在"才判定为
	// 版本缺资源。
	compat := false
	if err != nil {
		if !IsNotFoundError(err) {
			// 网络/授权等其他错误：如实上抛，不误判成版本差异。
			return false, fmt.Errorf("probing the textelements resource of %s: %w", t, err)
		}
		_ = resp
		// 资源 404。用宿主对象的存在性区分"版本缺资源"与"对象不存在"。
		if _, herr := c.transport.Request(ctx, t.objectURL(), &RequestOptions{
			Method: http.MethodGet,
			Accept: "*/*",
		}); herr != nil {
			if IsNotFoundError(herr) {
				return false, fmt.Errorf("%s does not exist", t)
			}
			return false, fmt.Errorf("probing the host object of %s: %w", t, herr)
		}
		if t.Type == "CLAS" {
			return false, fmt.Errorf("%s: this SAP release has no textelements resource and the RFC compat facade does not cover class text symbols; use SE24/SE80", t)
		}
		compat = true
	}

	c.compatText.rfc.Store(&compat)
	return compat, nil
}

// compatTextDocument 把 RFC 门面读回的完整池切成一个 kind 的文档，
// 复用现有的 plan/merge 逻辑。directives（@MaxLength 等）是 ADT 文档的
// 概念，经典 TEXTPOOL 没有对应物，这里一律为空——新增的 I 条目由
// textDocument.set 照常补上 @MaxLength。
func compatTextDocument(pool []TextPoolEntry, kindID string) *textDocument {
	doc := &textDocument{}
	for _, e := range pool {
		if e.ID != kindID {
			continue
		}
		doc.entries = append(doc.entries, textEntry{key: strings.TrimRight(e.Key, " "), text: e.Text})
	}
	return doc
}

// compatPoolApply 用本次修改后的若干 kind 文档重建完整池。
//
// 全池覆盖写回时，没被本次触碰的 kind（以及池里 VSP 不认识的条目，
// 例如经典 H 标题在老版本里的表示）必须原样保留——TEXTPOOL_SET 不是
// 补丁，漏带等于删除。同名 kind 的旧条目全部由修改后的文档替换。
// docs 的 key 是经典 kind ID（I/S/H），与 TextPoolEntry.ID 同一编码。
func compatPoolApply(pool []TextPoolEntry, docs map[string]*textDocument) []TextPoolEntry {
	out := make([]TextPoolEntry, 0, len(pool)+8)
	for _, e := range pool {
		if _, touch := docs[e.ID]; touch {
			continue // 该 kind 由修改后的文档整组替换
		}
		// 未触碰 kind / 未知 kind：原样保留（KEY 去掉 RFC 侧的填充空格）。
		out = append(out, TextPoolEntry{ID: e.ID, Key: strings.TrimRight(e.Key, " "), Text: e.Text})
	}
	for id, doc := range docs {
		for _, e := range doc.entries {
			out = append(out, TextPoolEntry{ID: id, Key: e.key, Text: e.text})
		}
	}
	return out
}

// compatTadirDevClass 从 TADIR 读对象所属开发包，供 TEXTPOOL_SET 的
// I_DEVCLASS 使用。与 MasterLanguage 一样走只读 SQL 预览；7.51 提供
// 该资源（datapreview 早于 ADT 表编辑器存在）。
func (c *Client) compatTadirDevClass(ctx context.Context, t TextPoolTarget) (string, error) {
	res, err := c.RunQuery(ctx, fmt.Sprintf(
		"SELECT devclass FROM tadir WHERE pgmid = 'R3TR' AND object = '%s' AND obj_name = '%s'",
		t.Type, sqlQuote(t.Name)), 1)
	if err != nil {
		return "", fmt.Errorf("reading the package of %s: %w", t, err)
	}
	if res == nil || len(res.Rows) == 0 {
		return "", fmt.Errorf("%s is not in TADIR", t)
	}
	return cell(res.Rows[0], "DEVCLASS"), nil
}

// writeTextPoolCompat 是 WriteTextPool 的兼容实现：
// 读完整池 → 按 kind 切文档、复用 plan/merge → 整池 RFC 写回 → 读回核验。
// 进入本函数前安全门、语言守卫与 plan 计算均已由调用方完成；
// docs 的 key 是 ADT 文档名（symbols/selections/headings）。
func (c *Client) writeTextPoolCompat(ctx context.Context, t TextPoolTarget, lang string, transport string, plan *TextPoolPlan, docs map[string]*textDocument) error {
	if c.compatFallback == nil {
		return fmt.Errorf("this SAP release does not offer the ADT textelements resource (7.52+), and no RFC compat facade (ZVSP_COMPAT_751) is configured")
	}

	// 文档名 → 经典 kind ID（symbols→I、selections→S）。
	changed := map[string]*textDocument{}
	for name, doc := range docs {
		changed[TextPoolKinds[name]] = doc
	}

	// 写回需要完整池：再读一次当前池，与 ADT 路径"锁下二读"的防漂移
	// 语义对应——两次读取之间若有人改了池，这里以最新读到的为准。
	pool, err := c.compatFallback.TextPoolGet(ctx, t.Name, lang)
	if err != nil {
		return fmt.Errorf("reading the text pool of %s via the compat facade: %w", t, err)
	}
	entries := compatPoolApply(pool, changed)

	devClass, err := c.compatTadirDevClass(ctx, t)
	if err != nil {
		return err
	}
	back, err := c.compatFallback.TextPoolSet(ctx, CompatTextPoolSet{
		Program:   t.Name,
		DevClass:  devClass,
		Transport: transport,
		Language:  lang,
		Entries:   entries,
	})
	if err != nil {
		return fmt.Errorf("writing the text pool of %s via the compat facade: %w", t, err)
	}
	// 经典仓库 API 直接写活动版本，没有 ADT 的"写后激活"两段式；
	// 读回条数放进计划备注，调用方据此确认写入生效。
	plan.Notes = append(plan.Notes,
		fmt.Sprintf("written via ZVSP_COMPAT_751 RFC; %d entries read back", len(back)))
	plan.Activated = true
	return nil
}

// compatFieldJSON 是 I_FIELDS_JSON 的一个元素，与 ABAP 端的字段协议对应
// （name/type/dataElement/len/dec/key/notNull/description）。
type compatFieldJSON struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	DataElement string `json:"dataElement,omitempty"`
	Len         int    `json:"len,omitempty"`
	Dec         int    `json:"dec,omitempty"`
	Key         bool   `json:"key,omitempty"`
	NotNull     bool   `json:"notNull,omitempty"`
	Description string `json:"description,omitempty"`
}

// compatFieldsJSON 把 TableField 列表编码成 ABAP 端约定好的字段 JSON。
// 内置类型集合与 mapFieldType 一致（额外补 CLNT/LANG/CUKY/UNIT）；
// 不认识的类型一律按数据元素名处理——这与 generateTableDDL 的兜底分支
// （"assume it's a data element name"）语义相同。
func compatFieldsJSON(fields []TableField) (string, error) {
	out := make([]compatFieldJSON, 0, len(fields))
	for _, f := range fields {
		cf := compatFieldJSON{
			Name:        strings.ToUpper(strings.TrimSpace(f.Name)),
			Len:         f.Length,
			Dec:         f.Decimals,
			Key:         f.IsKey,
			NotNull:     f.NotNull,
			Description: f.Description,
		}
		if cf.Name == "" {
			return "", fmt.Errorf("a field without a name is not allowed")
		}
		switch t := strings.ToUpper(strings.TrimSpace(f.Type)); t {
		case "CLNT", "MANDT", "CLIENT":
			cf.Type = "CLNT"
			if cf.Len == 0 {
				cf.Len = 3
			}
		case "DATE":
			cf.Type = "DATS" // 别名归一化，ABAP 端同样处理（双保险）
		case "TIME":
			cf.Type = "TIMS"
		case "":
			return "", fmt.Errorf("field %s: type or dataElement is required", cf.Name)
		default:
			// 内置类型码直接透传（CHAR/NUMC/RAW/DEC/CURR/QUAN/INT1..8/
			// FLTP/STRING/RAWSTRING/DATS/TIMS/LANG/CUKY/UNIT）；
			// 其余一律当作数据元素名，合法性由 ABAP 端校验。
			if isCompatBuiltinType(t) {
				cf.Type = t
			} else {
				cf.DataElement = t
			}
		}
		out = append(out, cf)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("encoding field JSON: %w", err)
	}
	return string(b), nil
}

// isCompatBuiltinType 列出 ABAP 端认得的 DDIC 内置类型码。
// STRING/RAWSTRING 的内部码 STRG/RSTR 由 ABAP 端翻译，wire 上保留人类可读值。
func isCompatBuiltinType(t string) bool {
	switch t {
	case "CHAR", "NUMC", "RAW", "DEC", "CURR", "QUAN",
		"INT1", "INT2", "INT4", "INT8", "FLTP",
		"STRING", "RAWSTRING", "DATS", "TIMS", "LANG", "CUKY", "UNIT":
		return true
	}
	return false
}
