package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleGetTypeInfoReturnsDataElementDocument(t *testing.T) {
	var gotPath, gotAccept string
	sap := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/vnd.sap.adt.dataelements.v2+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><blue:wbobj xmlns:blue="urn:wbobj" xmlns:adtcore="urn:core" xmlns:dtel="urn:dtel" adtcore:name="ZDEMO_PRICE" adtcore:description="Price"><dtel:dataElement><dtel:typeKind>domain</dtel:typeKind><dtel:typeName>ZDEMO_PRICE_DOMAIN</dtel:typeName><dtel:dataType>CURR</dtel:dataType><dtel:dataTypeLength>13</dtel:dataTypeLength><dtel:dataTypeDecimals>2</dtel:dataTypeDecimals></dtel:dataElement></blue:wbobj>`))
	}))
	defer sap.Close()

	srv := NewServer(&Config{BaseURL: sap.URL, Username: "TESTUSER", Client: "001", Mode: "expert"})
	result, err := srv.handleGetTypeInfo(t.Context(), newRequest(map[string]any{"type_name": "zdemo_price"}))
	if err != nil {
		t.Fatalf("handleGetTypeInfo: %v", err)
	}
	if result.IsError {
		t.Fatalf("handler returned error result: %s", resultText(result))
	}
	if gotPath != "/sap/bc/adt/ddic/dataelements/ZDEMO_PRICE" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAccept != "application/vnd.sap.adt.dataelements.v2+xml" {
		t.Errorf("Accept = %q", gotAccept)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(resultText(result)), &got); err != nil {
		t.Fatalf("handler output is not JSON: %v", err)
	}
	for key, want := range map[string]any{
		"Name": "ZDEMO_PRICE", "Type": "CURR", "Domain": "ZDEMO_PRICE_DOMAIN", "Description": "Price", "Length": float64(13), "Decimals": float64(2),
	} {
		if got[key] != want {
			t.Errorf("%s = %#v, want %#v; output: %s", key, got[key], want, resultText(result))
		}
	}
}
