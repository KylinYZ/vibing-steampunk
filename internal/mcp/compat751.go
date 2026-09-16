// compat751.go 把 Server 的共享经典 RFC 连接池接到 adt.CompatFallback：
// 老 SAP 版本（NetWeaver 7.51 及更早）缺失表创建与文本池 ADT 资源时，
// 由部署在目标系统上的 ZVSP_COMPAT_751 门面（abap/src/zvsp_compat）补齐。
// 参数编解码与结果判读都在 pkg/saprfc；这里只做类型适配与连接获取。
// 背景与版本边界见 docs/legacy-751-compat.md。
package mcp

import (
	"context"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// compat751Fallback 实现 adt.CompatFallback。RFC 未配置时
// s.rfcClientFor 返回错误，调用方据此回退到"资源缺失"的原语义。
type compat751Fallback struct {
	s *Server
}

// 编译期确认实现了接口。
var _ adt.CompatFallback = (*compat751Fallback)(nil)

// callCompat 实现 saprfc.CompatCaller：每次调用从共享连接池取客户端，
// keep-alive 与断线重拨由 rfcClientFor 统一负责。
func (f *compat751Fallback) callCompat(ctx context.Context, function string, params map[string]any) (saprfc.CompatResult, error) {
	c, release, err := f.s.rfcClientFor(ctx, nil)
	if err != nil {
		return saprfc.CompatResult{}, err
	}
	defer release()
	return saprfc.RFCCompatCaller{Client: c}.Call(ctx, function, params)
}

// toCompatFields 把 adt.TableField 翻成门面的字段协议。
// Type 里非内置的值（数据元素名）由 pkg/saprfc 兜底归类。
func toCompatFields(fields []adt.TableField) []saprfc.CompatField {
	out := make([]saprfc.CompatField, 0, len(fields))
	for _, fl := range fields {
		out = append(out, saprfc.CompatField{
			Name:        fl.Name,
			Type:        fl.Type,
			Length:      fl.Length,
			Decimals:    fl.Decimals,
			Key:         fl.IsKey,
			NotNull:     fl.NotNull,
			Description: fl.Description,
		})
	}
	return out
}

func (f *compat751Fallback) TableCreate(ctx context.Context, req adt.CompatTableCreate) error {
	return (&saprfc.Compat751{Caller: callerFunc(f.callCompat)}).TableCreate(ctx, saprfc.CompatTableCreateRequest{
		Name:          req.Name,
		Description:   req.Description,
		Package:       req.Package,
		Transport:     req.Transport,
		DeliveryClass: req.DeliveryClass,
		TableCategory: req.TableCategory,
		DataClass:     req.DataClass,
		Buffering:     req.Buffering,
		Fields:        toCompatFields(req.Fields),
	})
}

func (f *compat751Fallback) TextPoolGet(ctx context.Context, program, lang string) ([]adt.TextPoolEntry, error) {
	entries, err := (&saprfc.Compat751{Caller: callerFunc(f.callCompat)}).TextPoolGet(ctx, program, lang)
	if err != nil {
		return nil, err
	}
	out := make([]adt.TextPoolEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, adt.TextPoolEntry{ID: e.ID, Key: e.Key, Text: e.Text})
	}
	return out, nil
}

func (f *compat751Fallback) TextPoolSet(ctx context.Context, req adt.CompatTextPoolSet) ([]adt.TextPoolEntry, error) {
	entries := make([]saprfc.CompatTextEntry, 0, len(req.Entries))
	for _, e := range req.Entries {
		entries = append(entries, saprfc.CompatTextEntry{ID: e.ID, Key: e.Key, Text: e.Text})
	}
	back, err := (&saprfc.Compat751{Caller: callerFunc(f.callCompat)}).TextPoolSet(ctx, saprfc.CompatTextPoolSetRequest{
		Program:   req.Program,
		DevClass:  req.DevClass,
		Transport: req.Transport,
		Language:  req.Language,
		Entries:   entries,
	})
	if err != nil {
		return nil, err
	}
	out := make([]adt.TextPoolEntry, 0, len(back))
	for _, e := range back {
		out = append(out, adt.TextPoolEntry{ID: e.ID, Key: e.Key, Text: e.Text})
	}
	return out, nil
}

// callerFunc 把方法值包成 saprfc.CompatCaller 接口。
type callerFunc func(ctx context.Context, function string, params map[string]any) (saprfc.CompatResult, error)

func (f callerFunc) Call(ctx context.Context, function string, params map[string]any) (saprfc.CompatResult, error) {
	return f(ctx, function, params)
}
