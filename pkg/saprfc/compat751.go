package saprfc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KylinYZ/open-rfc-go/rfc"
)

// --- ZVSP_COMPAT_751 门面的 Go 侧驱动 ----------------------------------------
//
// 目标系统部署了 abap/src/zvsp_compat 的函数模块后，本文件把它的
// 封闭动作集（TABLE_CREATE / TEXTPOOL_GET / TEXTPOOL_SET）包成可编程调用。
// 这里只做参数组装与结果判读；ABAP 端负责 Z/Y 校验、传输归属、
// 授权检查与激活——两侧的约定见该目录 README 的"字段 JSON 协议"与
// "失败约定"。

// CompatCaller 是一次门面调用的抽象。生产实现经共享 RFC 连接池驱动
// open-rfc-go；测试注入内存假实现，不需要真实网关。
// Scalars 按导出参数名取值（E_RC 为 int32，E_MESSAGE 为 string），
// Tables 按表参数名取行（E_TEXTPOOL 的行是 ID/KEY/ENTRY 字符串）。
type CompatCaller interface {
	Call(ctx context.Context, function string, params map[string]any) (CompatResult, error)
}

// CompatResult 是门面调用的导出：按名取的标量与表行。
type CompatResult struct {
	Scalars map[string]any
	Tables  map[string][]map[string]any
}

// CompatTextEntry 是 TEXTPOOL 结构的一行（ID/KEY/ENTRY，经典文本池编码：
// I=文本符号、S=选择文本；本通道不维护的条目原样透传）。
type CompatTextEntry struct {
	ID   string
	Key  string
	Text string
}

// Compat751 驱动部署在目标系统上的兼容门面。Function 为空时用默认名。
type Compat751 struct {
	Caller   CompatCaller
	Function string
}

// DefaultCompatFunction 是门面函数模块的部署名。
const DefaultCompatFunction = "ZVSP_COMPAT_751"

func (f *Compat751) function() string {
	if f.Function != "" {
		return f.Function
	}
	return DefaultCompatFunction
}

// CompatField 是字段 JSON 协议的一个元素（见 abap README 的协议表）。
type CompatField struct {
	Name        string
	Type        string // DDIC 内置类型码；空且 DataElement 空会被 ABAP 端拒绝
	DataElement string // 数据元素名，给出时优先于 Type
	Length      int
	Decimals    int
	Key         bool
	NotNull     bool
	Description string
}

// CompatTableCreateRequest 是 TABLE_CREATE 的输入。
type CompatTableCreateRequest struct {
	Name          string
	Description   string
	Package       string
	Transport     string
	DeliveryClass string // 空 = ABAP 端补 A
	TableCategory string // 空 = TRANSP
	DataClass     string // 空 = APPL0
	Buffering     string // 空 = 不缓冲
	Language      string // 空 = ABAP 端用 SY-LANGU
	Fields        []CompatField
}

// CompatTextPoolSetRequest 是 TEXTPOOL_SET 的输入。
type CompatTextPoolSetRequest struct {
	Program   string
	DevClass  string
	Transport string
	Language  string
	Entries   []CompatTextEntry
}

// compatBuiltinTypes 是 ABAP 端认得的 DDIC 内置类型码；不在此列的类型
// 会被当作数据元素名。STRING/RAWSTRING 的内部码（STRG/RSTR）由 ABAP 端
// 翻译，wire 上保留人类可读值。
var compatBuiltinTypes = map[string]bool{
	"CHAR": true, "NUMC": true, "RAW": true, "DEC": true, "CURR": true,
	"QUAN": true, "INT1": true, "INT2": true, "INT4": true, "INT8": true,
	"FLTP": true, "STRING": true, "RAWSTRING": true, "DATS": true,
	"TIMS": true, "LANG": true, "CUKY": true, "UNIT": true,
}

// TableCreate 调 TABLE_CREATE：创建并激活一张透明表。
// 返回 nil 即建表且激活成功；激活失败以含 e_activation_rc 的错误返回，
// 会留下未激活的 DDIC 定义（ABAP 端不自动删除，需要 SE11 人工处理）。
func (f *Compat751) TableCreate(ctx context.Context, req CompatTableCreateRequest) error {
	fields := make([]map[string]any, 0, len(req.Fields))
	for _, fl := range req.Fields {
		name := strings.ToUpper(strings.TrimSpace(fl.Name))
		if name == "" {
			return fmt.Errorf("compat table create: a field without a name is not allowed")
		}
		row := map[string]any{
			"name": name,
			"key":  fl.Key,
			// /ui2/cl_json 省略 false 字段不影响语义，显式写出更利于排障。
			"notNull": fl.NotNull,
		}
		if fl.Description != "" {
			row["description"] = fl.Description
		}
		// 数据元素优先于类型；类型槽里给了非内置值时按数据元素兜底，
		// 与 ADT 路径 generateTableDDL 的兜底语义一致。
		de := strings.ToUpper(strings.TrimSpace(fl.DataElement))
		t := strings.ToUpper(strings.TrimSpace(fl.Type))
		switch {
		case de != "":
			row["dataElement"] = de
		case t == "":
			return fmt.Errorf("compat table create: field %s needs a type or a data element", name)
		case t == "CLIENT" || t == "MANDT" || t == "CLNT":
			row["type"] = "CLNT"
			row["len"] = orDefault(fl.Length, 3)
		case t == "DATE":
			row["type"] = "DATS" // 别名归一化，ABAP 端同样处理（双保险）
		case t == "TIME":
			row["type"] = "TIMS"
		case compatBuiltinTypes[t]:
			row["type"] = t
			if fl.Length > 0 {
				row["len"] = fl.Length
			}
			if fl.Decimals > 0 {
				row["dec"] = fl.Decimals
			}
		default:
			row["dataElement"] = t
		}
		fields = append(fields, row)
	}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("compat table create: encoding field JSON: %w", err)
	}

	params := map[string]any{
		"I_OP":          "TABLE_CREATE",
		"I_TABLE":       req.Name,
		"I_DESCRIPTION": req.Description,
		"I_DEVCLASS":    req.Package,
		"I_TRANSPORT":   req.Transport,
		"I_FIELDS_JSON": string(fieldsJSON),
		"I_TABART":      req.DataClass,
		"I_BUFFERING":   req.Buffering,
	}
	if req.DeliveryClass != "" {
		params["I_DELIVERY_CLASS"] = req.DeliveryClass
	}
	if req.TableCategory != "" {
		params["I_TABLE_CATEGORY"] = req.TableCategory
	}
	if req.Language != "" {
		params["I_LANGUAGE"] = req.Language
	}

	res, err := f.Caller.Call(ctx, f.function(), params)
	if err != nil {
		// RFC 层失败：连接问题，或门面尚未部署（FM 不存在以
		// SYSTEM_FAILURE/异常形式冒出）。错误里带上函数名便于定位。
		return fmt.Errorf("calling %s(TABLE_CREATE): %w", f.function(), err)
	}
	return compatFailure(f.function(), res)
}

// TextPoolGet 调 TEXTPOOL_GET：读回程序在指定语言的完整文本池。
// 池里可能含本通道不维护的条目（经典 H 标题等），原样返回。
func (f *Compat751) TextPoolGet(ctx context.Context, program, lang string) ([]CompatTextEntry, error) {
	params := map[string]any{
		"I_OP":       "TEXTPOOL_GET",
		"I_PROGRAM":  program,
		"I_LANGUAGE": lang,
	}
	res, err := f.Caller.Call(ctx, f.function(), params)
	if err != nil {
		return nil, fmt.Errorf("calling %s(TEXTPOOL_GET): %w", f.function(), err)
	}
	if rc := compatInt(res.Scalars["E_RC"]); rc != 0 {
		return nil, compatFailure(f.function(), res)
	}
	return compatEntries(res.Tables["E_TEXTPOOL"]), nil
}

// TextPoolSet 调 TEXTPOOL_SET：以完整 TEXTPOOL 覆盖写入，并返回 ABAP 端
// 读回的活动版本池供核验。Entries 必须是合并后的完整池。
func (f *Compat751) TextPoolSet(ctx context.Context, req CompatTextPoolSetRequest) ([]CompatTextEntry, error) {
	rows := make([]map[string]any, 0, len(req.Entries))
	for _, e := range req.Entries {
		rows = append(rows, map[string]any{
			"ID":    e.ID,
			"KEY":   e.Key,
			"ENTRY": e.Text,
		})
	}
	params := map[string]any{
		"I_OP":        "TEXTPOOL_SET",
		"I_PROGRAM":   req.Program,
		"I_DEVCLASS":  req.DevClass,
		"I_TRANSPORT": req.Transport,
		"I_LANGUAGE":  req.Language,
		// TABLES 参数按行 map 传入；open-rfc-go 按 TEXTPOOL 的 DDIC 布局
		// 固定宽编码，缺省字段补初始值。
		"I_TEXTPOOL": rows,
	}
	res, err := f.Caller.Call(ctx, f.function(), params)
	if err != nil {
		return nil, fmt.Errorf("calling %s(TEXTPOOL_SET): %w", f.function(), err)
	}
	if rc := compatInt(res.Scalars["E_RC"]); rc != 0 {
		return nil, compatFailure(f.function(), res)
	}
	return compatEntries(res.Tables["E_TEXTPOOL"]), nil
}

// compatEntries 把 E_TEXTPOOL 行转换成条目，去掉 RFC 解码侧 CHAR 字段的
// 填充空格。
func compatEntries(rows []map[string]any) []CompatTextEntry {
	out := make([]CompatTextEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, CompatTextEntry{
			ID:   compatString(r["ID"]),
			Key:  strings.TrimRight(compatString(r["KEY"]), " "),
			Text: compatString(r["ENTRY"]),
		})
	}
	return out
}

// compatFailure 把非零 E_RC 翻译成错误；E_RC 为 0 返回 nil，因此成功
// 路径也可以直接把结果交给它判读。激活失败（activation_rc>4）单独点名，
// 因为那意味着存在需要人工处理的未激活定义。
func compatFailure(function string, res CompatResult) error {
	msg := compatString(res.Scalars["E_MESSAGE"])
	rc := compatInt(res.Scalars["E_RC"])
	act := compatInt(res.Scalars["E_ACTIVATION_RC"])
	if act > 4 {
		return fmt.Errorf("%s failed (rc=%d, activation_rc=%d): %s — an inactive DDIC definition was left behind; review it in SE11", function, rc, act, msg)
	}
	if rc == 0 {
		return nil
	}
	if msg == "" {
		return fmt.Errorf("%s failed (rc=%d)", function, rc)
	}
	return fmt.Errorf("%s failed (rc=%d): %s", function, rc, msg)
}

// compatInt 把 RFC 解码侧的整数导出归一为 int（open-rfc-go 的 INT4 解码
// 成 int32，JSON 侧可能是 float64）。
func compatInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func compatString(v any) string {
	s, _ := v.(string)
	return s
}

func orDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

// RFCCompatCaller 把 open-rfc-go 客户端适配成 CompatCaller。导出参数按
// 门面的已知接口点名取回——门面的导出集合是封闭的（E_RC/E_ACTIVATION_RC/
// E_MESSAGE + E_TEXTPOOL），无需通用枚举。
type RFCCompatCaller struct{ Client *rfc.Client }

func (c RFCCompatCaller) Call(ctx context.Context, function string, params map[string]any) (CompatResult, error) {
	res, err := c.Client.Call(ctx, function, rfc.Params(params))
	if err != nil {
		return CompatResult{}, err
	}
	return CompatResult{
		Scalars: map[string]any{
			"E_RC":            res.Get("E_RC"),
			"E_ACTIVATION_RC": res.Get("E_ACTIVATION_RC"),
			"E_MESSAGE":       res.Get("E_MESSAGE"),
		},
		Tables: map[string][]map[string]any{
			"E_TEXTPOOL": res.Table("E_TEXTPOOL"),
		},
	}, nil
}
