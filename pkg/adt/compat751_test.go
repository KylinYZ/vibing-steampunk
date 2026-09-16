package adt

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeCompatFallback 记录兼容门面的每次调用，可注入失败，用来断言
// CreateTable / 文本池在老系统上确实改走了 RFC 通道、参数正确、
// 合并语义无损。
type fakeCompatFallback struct {
	mu sync.Mutex

	// 记录
	tableCalls []CompatTableCreate
	getCalls   [][2]string // [program, lang]
	setCalls   []CompatTextPoolSet

	// 注入行为
	tableErr error
	getErr   error
	setErr   error
	pool     []TextPoolEntry // TextPoolGet 的返回
	setBack  []TextPoolEntry // TextPoolSet 的读回返回
}

func (f *fakeCompatFallback) TableCreate(ctx context.Context, req CompatTableCreate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tableCalls = append(f.tableCalls, req)
	return f.tableErr
}

func (f *fakeCompatFallback) TextPoolGet(ctx context.Context, program, lang string) ([]TextPoolEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, [2]string{program, lang})
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.pool, nil
}

func (f *fakeCompatFallback) TextPoolSet(ctx context.Context, req CompatTextPoolSet) ([]TextPoolEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls = append(f.setCalls, req)
	if f.setErr != nil {
		return nil, f.setErr
	}
	return f.setBack, nil
}

// compatProbeServer 模拟一台 7.51：/ddic/tables 的 POST 与 textelements
// 资源一律 404，其余对象（宿主程序、CSRF 探测）正常应答。
type compatProbeServer struct {
	mu       sync.Mutex
	requests []string
	// tableStatus 决定建表 POST 的应答，用于测非 404 不分流。
	tableStatus int
}

func (s *compatProbeServer) start(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		s.mu.Lock()
		s.requests = append(s.requests, r.Method+" "+path)
		s.mu.Unlock()

		switch {
		case strings.Contains(path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)
		case strings.Contains(path, "/ddic/tables") && r.Method == http.MethodPost:
			// tableStatus 为零值（未设置）按 404 处理——WriteHeader(0) 会 panic。
			status := s.tableStatus
			if status == 0 {
				status = http.StatusNotFound
			}
			w.WriteHeader(status)
		case strings.Contains(path, "/textelements"):
			// 7.51 没有 textelements 资源集合：对象级与文档级都 404。
			w.WriteHeader(http.StatusNotFound)
		case strings.Contains(path, "/programs/programs/"):
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)

	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func (s *compatProbeServer) saw(method, pathPart string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.requests {
		if strings.HasPrefix(r, method+" ") && strings.Contains(r, pathPart) {
			return true
		}
	}
	return false
}

// CreateTable 在 ADT 建表资源 404（7.51）时改走 RFC 门面：字段前置客户端
// 字段，包与传输请求原样传递，ADT 404 不再向外抛。
func TestCreateTableFallsBackToCompatOn404(t *testing.T) {
	srv := &compatProbeServer{}
	client := srv.start(t)
	fake := &fakeCompatFallback{}
	client.SetCompatFallback(fake)

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name:        "zdemo_compat",
		Description: "demo",
		Package:     "$TMP",
		Fields: []TableField{
			{Name: "FIELD1", Type: "CHAR", Length: 10, IsKey: true},
		},
	})
	if err != nil {
		t.Fatalf("compat create failed: %v", err)
	}
	if len(fake.tableCalls) != 1 {
		t.Fatalf("table create called %d times, want 1", len(fake.tableCalls))
	}
	call := fake.tableCalls[0]
	if call.Name != "ZDEMO_COMPAT" || call.Package != "$TMP" || call.DeliveryClass != "A" {
		t.Errorf("request: %+v", call)
	}
	if len(call.Fields) != 2 {
		t.Fatalf("fields: %+v", call.Fields)
	}
	// 客户端字段由 Go 端前置，与 generateTableDDL 的 `key client` 对应。
	if call.Fields[0].Name != "MANDT" || call.Fields[0].Type != "CLNT" || !call.Fields[0].IsKey || !call.Fields[0].NotNull {
		t.Errorf("client field: %+v", call.Fields[0])
	}
	if call.Fields[1].Name != "FIELD1" || call.Fields[1].Type != "CHAR" || call.Fields[1].Length != 10 {
		t.Errorf("field 1: %+v", call.Fields[1])
	}
}

// 调用方自己给了客户端字段时不重复前置——否则 RPY_TABLE_INSERT 会看到
// 两个键位相同的 CLNT 字段。
func TestPrependClientFieldSkipsExistingClientField(t *testing.T) {
	fields := []TableField{{Name: "CLNT", Type: "MANDT", Length: 3, IsKey: true}}
	got := prependClientField(fields)
	if len(got) != 1 || got[0].Name != "CLNT" {
		t.Errorf("client alias not honored: %+v", got)
	}
	got = prependClientField([]TableField{{Name: "A", Type: "CHAR", Length: 1}})
	if len(got) != 2 || got[0].Name != "MANDT" || got[0].Type != "CLNT" {
		t.Errorf("client field not prepended: %+v", got)
	}
}

// 没有 RFC 门面时 404 不再无声无息：错误里同时给出版本边界与门面名。
func TestCreateTableWithoutFacadeExplains404(t *testing.T) {
	srv := &compatProbeServer{}
	client := srv.start(t)

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []TableField{{Name: "F", Type: "CHAR", Length: 1}},
	})
	if err == nil {
		t.Fatal("expected the missing-facade error")
	}
	for _, want := range []string{"7.52", "ZVSP_COMPAT_751"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q misses %q", err, want)
		}
	}
}

// 只有 404（资源不存在）触发兼容分流；参数错误等保持原语义。
func TestCreateTableDoesNotFallbackOnOtherErrors(t *testing.T) {
	srv := &compatProbeServer{tableStatus: http.StatusBadRequest}
	client := srv.start(t)
	fake := &fakeCompatFallback{}
	client.SetCompatFallback(fake)

	err := client.CreateTable(context.Background(), CreateTableOptions{
		Name: "ZDEMO", Package: "$TMP",
		Fields: []TableField{{Name: "F", Type: "CHAR", Length: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "creating table object") {
		t.Fatalf("expected the plain ADT error, got %v", err)
	}
	if len(fake.tableCalls) != 0 {
		t.Errorf("compat facade must not be called on a non-404: %+v", fake.tableCalls)
	}
}

// textPoolNeedsCompat 用对象级探测区分"版本缺资源"与"对象不存在"，
// 结论按 client 记忆（第二次调用不再发请求）。
func TestTextPoolNeedsCompatProbesOnce(t *testing.T) {
	srv := &compatProbeServer{}
	client := srv.start(t)

	tgt := TextPoolTarget{Type: "PROG", Name: "ZDEMO"}
	compat, err := client.textPoolNeedsCompat(context.Background(), tgt)
	if err != nil || !compat {
		t.Fatalf("compat=%v err=%v", compat, err)
	}
	if !srv.saw(http.MethodGet, "/textelements/programs/zdemo") ||
		!srv.saw(http.MethodGet, "/programs/programs/zdemo") {
		t.Fatalf("probe requests missing: %v", srv.requests)
	}
	n := len(srv.requests)
	if compat, err = client.textPoolNeedsCompat(context.Background(), tgt); err != nil || !compat {
		t.Fatalf("cached compat=%v err=%v", compat, err)
	}
	if len(srv.requests) != n {
		t.Errorf("probe repeated: %v", srv.requests)
	}
}

// 宿主对象也不存在时，404 是"对象不存在"，必须如实报错而不是路由。
func TestTextPoolNeedsCompatReportsMissingObject(t *testing.T) {
	host404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer host404.Close()
	cfg := NewConfig(host404.URL, "TESTUSER", "secret")
	c2 := NewClientWithTransport(cfg, NewTransport(cfg))

	_, err := c2.textPoolNeedsCompat(context.Background(), TextPoolTarget{Type: "PROG", Name: "ZGHOST"})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected a does-not-exist error, got %v", err)
	}
}

// ADT 资源正常应答的系统（7.52+）不走兼容通道。这是回归测试：探测判定
// 曾写反过一次，200 被当成了缺资源。
func TestTextPoolNeedsCompatFalseWhenResourceAnswers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/discovery") {
			w.Header().Set("X-CSRF-Token", "TOKEN")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	cfg := NewConfig(srv.URL, "TESTUSER", "secret")
	client := NewClientWithTransport(cfg, NewTransport(cfg))

	compat, err := client.textPoolNeedsCompat(context.Background(), TextPoolTarget{Type: "PROG", Name: "ZDEMO"})
	if err != nil || compat {
		t.Fatalf("compat=%v err=%v", compat, err)
	}
}

// compat 系统上的读取走 RFC 门面：GetTextPoolInLanguage 返回门面给的
// 完整池，而不是三个 404 静默拼出的空池。
func TestGetTextPoolInLanguageRoutesToCompat(t *testing.T) {
	srv := &compatProbeServer{}
	client := srv.start(t)
	fake := &fakeCompatFallback{pool: []TextPoolEntry{
		{ID: "I", Key: "001", Text: "Options"},
		{ID: "S", Key: "P_CARRID", Text: "Carrier"},
	}}
	client.SetCompatFallback(fake)

	entries, err := client.GetTextPoolInLanguage(context.Background(), "ZDEMO", "EN")
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if len(entries) != 2 || entries[0].Key != "001" || entries[1].Key != "P_CARRID" {
		t.Errorf("entries: %+v", entries)
	}
	if len(fake.getCalls) != 1 || fake.getCalls[0][0] != "ZDEMO" {
		t.Errorf("get calls: %v", fake.getCalls)
	}
}

// compatTextDocument 按 kind 切池并去掉 KEY 的填充空格。
func TestCompatTextDocument(t *testing.T) {
	pool := []TextPoolEntry{
		{ID: "I", Key: "001   ", Text: "Options"},
		{ID: "S", Key: "P_DEEP", Text: "Follow includes"},
	}
	doc := compatTextDocument(pool, "I")
	if len(doc.entries) != 1 || doc.entries[0].key != "001" || doc.entries[0].text != "Options" {
		t.Errorf("doc: %+v", doc.entries)
	}
}

// 全池覆盖写回的合并语义：触碰的 kind 整组替换，未知/未触碰 kind 原样
// 保留（漏带等于删除）。
func TestCompatPoolApply(t *testing.T) {
	pool := []TextPoolEntry{
		{ID: "I", Key: "001", Text: "old"},
		{ID: "S", Key: "P_DEEP", Text: "Follow includes"},
		{ID: "H", Key: "LISTHEADR", Text: "Kept"}, // 未知 kind 原样保留
	}
	docs := map[string]*textDocument{
		"I": {entries: []textEntry{{key: "001", text: "new"}, {key: "002", text: "added"}}},
	}
	got := compatPoolApply(pool, docs)
	if len(got) != 4 {
		t.Fatalf("applied: %+v", got)
	}
	if got[0].ID != "S" || got[0].Text != "Follow includes" {
		t.Errorf("untouched kind: %+v", got[0])
	}
	if got[1].ID != "H" || got[1].Text != "Kept" {
		t.Errorf("unknown kind: %+v", got[1])
	}
	if got[2].Key != "001" || got[2].Text != "new" || got[3].Key != "002" {
		t.Errorf("replaced kind: %+v %+v", got[2], got[3])
	}
}

// 字段 JSON 的翻译：内置类型直传、别名归一、数据元素兜底、缺类型报错。
func TestCompatFieldsJSON(t *testing.T) {
	fields := []TableField{
		{Name: "mandt", Type: "MANDT"},
		{Name: "name", Type: "CHAR", Length: 20, IsKey: true, NotNull: true, Description: "Name"},
		{Name: "amt", Type: "DEC", Length: 15, Decimals: 2},
		{Name: "created", Type: "created_at"}, // 非内置 → 数据元素兜底
	}
	j, err := compatFieldsJSON(fields)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	for _, want := range []string{
		`"name":"MANDT","type":"CLNT","len":3`,
		`"name":"NAME","type":"CHAR","len":20,"key":true,"notNull":true,"description":"Name"`,
		`"name":"AMT","type":"DEC","len":15,"dec":2`,
		`"name":"CREATED","dataElement":"CREATED_AT"`,
	} {
		if !strings.Contains(j, want) {
			t.Errorf("json %s misses %s", j, want)
		}
	}
	if _, err := compatFieldsJSON([]TableField{{Name: "X", Type: ""}}); err == nil {
		t.Error("a field without a type was accepted")
	}
	if _, err := compatFieldsJSON([]TableField{{Type: "CHAR"}}); err == nil {
		t.Error("a field without a name was accepted")
	}
}
