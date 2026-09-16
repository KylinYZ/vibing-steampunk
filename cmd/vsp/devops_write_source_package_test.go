package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestRunSourceWriteResolvesExistingPackage exercises the CLI entry rather
// than WriteSource directly. An existing object must not need --package (the
// CLI has none); its package comes from ADT metadata before the write lock.
func TestRunSourceWriteResolvesExistingPackage(t *testing.T) {
	var paths []string
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("X-CSRF-Token", "test-token")
		switch {
		case strings.Contains(r.URL.Path, "informationsystem/search"):
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="/sap/bc/adt/programs/programs/zdemo_cli_package" adtcore:type="PROG/P" adtcore:name="ZDEMO_CLI_PACKAGE" adtcore:packageName="$TMP"/>
</adtcore:objectReferences>`)
		case strings.Contains(r.URL.Path, "/checkruns"):
			w.Header().Set("Content-Type", "application/vnd.sap.adt.checkmessages+xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><chkl:messages xmlns:chkl="http://www.sap.com/adt/checklist"/>`)
		case r.URL.Query().Get("_action") == "LOCK":
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA><LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><IS_LOCAL>X</IS_LOCAL></DATA></asx:values></asx:abap>`)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer sap.Close()

	t.Setenv("SAP_URL", sap.URL)
	t.Setenv("SAP_USER", "TESTUSER")
	t.Setenv("SAP_PASSWORD", "secret")
	// 本测试带 --transport 验证包解析流程；显式声明 transportable opt-in，
	// 不依赖命令行 flag 或其他测试残留的全局状态（单跑/组合跑行为一致）。
	t.Setenv("SAP_ALLOW_TRANSPORTABLE_EDITS", "true")
	t.Setenv("SAP_ALLOWED_PACKAGES", "$TMP")
	oldSystemName := systemName
	systemName = ""
	t.Cleanup(func() { systemName = oldSystemName })

	stdin, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "REPORT zdemo_cli_package."); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	oldStdin := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = oldStdin; _ = stdin.Close() })

	// 本测试直接调用孤立的 sourceWriteCmd（未挂到 root 命令树下），
	// 而 resolveAllowTransportableEdits 会 fail-closed 地查找该 flag；
	// 显式挂上 root 的 persistent flag 以模拟真实命令树。
	if sourceWriteCmd.Flags().Lookup("allow-transportable-edits") == nil {
		sourceWriteCmd.Flags().AddFlag(rootCmd.PersistentFlags().Lookup("allow-transportable-edits"))
	}

	err = runSourceWrite(sourceWriteCmd, []string{"PROG", "ZDEMO_CLI_PACKAGE"})
	if err != nil {
		t.Fatalf("source write: %v", err)
	}
	seenSearch, seenLock := false, false
	for _, path := range paths {
		seenSearch = seenSearch || strings.Contains(path, "informationsystem/search")
		seenLock = seenLock || strings.Contains(path, "/programs/programs/")
	}
	if !seenSearch || !seenLock {
		t.Fatalf("CLI write did not resolve package then enter update flow: %v", paths)
	}
}
