package saprfc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// fakeCompatCaller 记录调用并按脚本应答，让参数编解码与结果判读
// 可以在没有网关的环境下完整测试。
type fakeCompatCaller struct {
	mu      sync.Mutex
	calls   []fakeCompatCall
	results map[string]CompatResult // 按动作（TABLE_CREATE 等）应答
	err     error                   // RFC 层失败（连接、FM 不存在）
}

type fakeCompatCall struct {
	function string
	params   map[string]any
}

func (c *fakeCompatCaller) Call(ctx context.Context, function string, params map[string]any) (CompatResult, error) {
	c.mu.Lock()
	c.calls = append(c.calls, fakeCompatCall{function: function, params: params})
	res, has := c.results[params["I_OP"].(string)]
	c.mu.Unlock()
	if c.err != nil {
		return CompatResult{}, c.err
	}
	if !has {
		return CompatResult{Scalars: map[string]any{"E_RC": int32(0)}}, nil
	}
	return res, nil
}

func (c *fakeCompatCaller) last() fakeCompatCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[len(c.calls)-1]
}

// 字段 JSON：内置类型直传、CLNT 补长度、别名归一、非内置类型按数据元素
// 兜底——这是与 ABAP 端协议的直接契约。
func TestCompatTableCreateEncodesFields(t *testing.T) {
	fake := &fakeCompatCaller{}
	f := &Compat751{Caller: fake}
	err := f.TableCreate(context.Background(), CompatTableCreateRequest{
		Name:        "ZDEMO",
		Description: "demo",
		Package:     "$TMP",
		Fields: []CompatField{
			{Name: "MANDT", Type: "MANDT", Key: true},
			{Name: "NAME", Type: "CHAR", Length: 20, Key: true, NotNull: true, Description: "Name"},
			{Name: "AMT", Type: "DEC", Length: 15, Decimals: 2},
			{Name: "CARrname", DataElement: "s_carrname"},
			{Name: "TXT", Type: "created_by"}, // 类型槽给了非内置值 → 数据元素兜底
			{Name: "BORN", Type: "DATE"},
		},
	})
	if err != nil {
		t.Fatalf("table create: %v", err)
	}
	call := fake.last()
	if call.function != DefaultCompatFunction || call.params["I_OP"] != "TABLE_CREATE" || call.params["I_TABLE"] != "ZDEMO" {
		t.Errorf("params: %+v", call.params)
	}
	var fields []map[string]any
	if err := json.Unmarshal([]byte(call.params["I_FIELDS_JSON"].(string)), &fields); err != nil {
		t.Fatalf("fields json: %v", err)
	}
	type expect struct {
		name string
		key  string // JSON 键（type/dataElement）
		val  any
	}
	checks := []expect{
		{"MANDT", "type", "CLNT"},
		{"NAME", "len", float64(20)},
		{"AMT", "dec", float64(2)},
		{"CARRNAME", "dataElement", "S_CARRNAME"},
		{"TXT", "dataElement", "CREATED_BY"},
		{"BORN", "type", "DATS"},
	}
	byName := map[string]map[string]any{}
	for _, fl := range fields {
		byName[fl["name"].(string)] = fl
	}
	for _, c := range checks {
		fl := byName[c.name]
		if fl == nil || fl[c.key] != c.val {
			t.Errorf("field %s: want %s=%v, got %v", c.name, c.key, c.val, fl)
		}
	}
	if byName["MANDT"]["len"] != float64(3) {
		t.Errorf("client length: %v", byName["MANDT"]["len"])
	}
}

// 缺类型也缺数据元素的字段在 Go 侧就被拒绝，不浪费一次 RFC 往返。
func TestCompatTableCreateRejectsUntypedField(t *testing.T) {
	fake := &fakeCompatCaller{}
	f := &Compat751{Caller: fake}
	err := f.TableCreate(context.Background(), CompatTableCreateRequest{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []CompatField{{Name: "X"}},
	})
	if err == nil || !strings.Contains(err.Error(), "type or a data element") {
		t.Fatalf("err: %v", err)
	}
}

// 失败判读：E_RC 带消息；激活失败单独点名"留下了未激活定义"。
func TestCompatFailureSemantics(t *testing.T) {
	fake := &fakeCompatCaller{results: map[string]CompatResult{
		"TABLE_CREATE": {Scalars: map[string]any{
			"E_RC": int32(8), "E_MESSAGE": "already exist",
		}},
	}}
	f := &Compat751{Caller: fake}
	err := f.TableCreate(context.Background(), CompatTableCreateRequest{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []CompatField{{Name: "A", Type: "CHAR", Length: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "rc=8") || !strings.Contains(err.Error(), "already exist") {
		t.Fatalf("err: %v", err)
	}

	fake2 := &fakeCompatCaller{results: map[string]CompatResult{
		"TABLE_CREATE": {Scalars: map[string]any{
			"E_RC": int32(12), "E_ACTIVATION_RC": int32(8), "E_MESSAGE": "check failed",
		}},
	}}
	err = (&Compat751{Caller: fake2}).TableCreate(context.Background(), CompatTableCreateRequest{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []CompatField{{Name: "A", Type: "CHAR", Length: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "inactive DDIC definition") || !strings.Contains(err.Error(), "activation_rc=8") {
		t.Fatalf("activation err: %v", err)
	}

	// RFC 层失败（典型：门面未部署）原样带出。
	fake3 := &fakeCompatCaller{err: fmt.Errorf("SYSTEM_FAILURE: ZVSP_COMPAT_751 not found")}
	err = (&Compat751{Caller: fake3}).TableCreate(context.Background(), CompatTableCreateRequest{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []CompatField{{Name: "A", Type: "CHAR", Length: 1}},
	})
	if err == nil || !errors.Is(err, err) || !strings.Contains(err.Error(), "TABLE_CREATE") {
		t.Fatalf("rfc err: %v", err)
	}
}

// 文本池读写往返：SET 的行按 ID/KEY/ENTRY 组装，GET/SET 的读回去掉 KEY
// 的填充空格。
func TestCompatTextPoolRoundTrip(t *testing.T) {
	fake := &fakeCompatCaller{results: map[string]CompatResult{
		"TEXTPOOL_GET": {
			Scalars: map[string]any{"E_RC": int32(0)},
			Tables: map[string][]map[string]any{
				"E_TEXTPOOL": {
					{"ID": "I", "KEY": "001   ", "ENTRY": "Options"},
					{"ID": "S", "KEY": "P_DEEP", "ENTRY": "Follow includes"},
				},
			},
		},
		"TEXTPOOL_SET": {
			Scalars: map[string]any{"E_RC": int32(0)},
			Tables: map[string][]map[string]any{
				"E_TEXTPOOL": {{"ID": "I", "KEY": "001", "ENTRY": "written"}},
			},
		},
	}}
	f := &Compat751{Caller: fake}

	got, err := f.TextPoolGet(context.Background(), "ZDEMO", "EN")
	if err != nil || len(got) != 2 || got[0].Key != "001" {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	back, err := f.TextPoolSet(context.Background(), CompatTextPoolSetRequest{
		Program: "ZDEMO", DevClass: "$TMP", Transport: "A4HK900123", Language: "EN",
		Entries: []CompatTextEntry{{ID: "I", Key: "001", Text: "written"}},
	})
	if err != nil || len(back) != 1 || back[0].Text != "written" {
		t.Fatalf("set: %+v err=%v", back, err)
	}
	call := fake.last()
	if call.params["I_PROGRAM"] != "ZDEMO" || call.params["I_DEVCLASS"] != "$TMP" || call.params["I_TRANSPORT"] != "A4HK900123" {
		t.Errorf("set params: %+v", call.params)
	}
	rows := call.params["I_TEXTPOOL"].([]map[string]any)
	if len(rows) != 1 || rows[0]["ID"] != "I" || rows[0]["KEY"] != "001" || rows[0]["ENTRY"] != "written" {
		t.Errorf("set rows: %+v", rows)
	}
}

// E_RC 非零时 SET 失败，读回值为空。
func TestCompatTextPoolSetFailure(t *testing.T) {
	fake := &fakeCompatCaller{results: map[string]CompatResult{
		"TEXTPOOL_SET": {Scalars: map[string]any{"E_RC": int32(2), "E_MESSAGE": "permission error"}},
	}}
	back, err := (&Compat751{Caller: fake}).TextPoolSet(context.Background(), CompatTextPoolSetRequest{
		Program: "ZDEMO", DevClass: "$TMP",
		Entries: []CompatTextEntry{{ID: "I", Key: "001", Text: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "permission error") || back != nil {
		t.Fatalf("err=%v back=%v", err, back)
	}
}
