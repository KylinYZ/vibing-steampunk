package adt

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetTypeInfoUsesDataElementDocument(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		body     string
		want     TypeInfo
		wantPath string
	}{
		{
			name:     "predefined fixed type with alternate prefixes",
			input:    "zdemo_code",
			body:     `<?xml version="1.0"?><dictionary:wbobj xmlns:dictionary="http://www.sap.com/wbobj/dictionary/dtel" xmlns:core="http://www.sap.com/adt/core" xmlns:element="http://www.sap.com/adt/dictionary/dataelements" core:name="ZDEMO_CODE" core:description="Code &amp; description"><element:dataElement><element:typeKind>predefinedAbapType</element:typeKind><element:typeName>CHAR</element:typeName><element:dataType>CHAR</element:dataType><element:dataTypeLength>18</element:dataTypeLength><element:dataTypeDecimals>0</element:dataTypeDecimals></element:dataElement></dictionary:wbobj>`,
			want:     TypeInfo{Name: "ZDEMO_CODE", Type: "CHAR", Description: "Code & description", Length: 18, Decimals: 0},
			wantPath: "/sap/bc/adt/ddic/dataelements/ZDEMO_CODE",
		},
		{
			name:     "domain reference keeps the resolved data type",
			input:    "/demo/amount",
			body:     `<?xml version="1.0"?><blue:wbobj xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:adtcore="http://www.sap.com/adt/core" xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements" adtcore:name="/DEMO/AMOUNT" adtcore:description="Amount"><dtel:dataElement><dtel:typeKind>domain</dtel:typeKind><dtel:typeName>ZDEMO_AMOUNT_DOMAIN</dtel:typeName><dtel:dataType>DEC</dtel:dataType><dtel:dataTypeLength>11</dtel:dataTypeLength><dtel:dataTypeDecimals>2</dtel:dataTypeDecimals></dtel:dataElement></blue:wbobj>`,
			want:     TypeInfo{Name: "/DEMO/AMOUNT", Type: "DEC", Domain: "ZDEMO_AMOUNT_DOMAIN", Description: "Amount", Length: 11, Decimals: 2},
			wantPath: "/sap/bc/adt/ddic/dataelements/%2FDEMO%2FAMOUNT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAccept, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotAccept = r.Header.Get("Accept")
				gotPath = r.URL.EscapedPath()
				w.Header().Set("Content-Type", "application/vnd.sap.adt.dataelements.v2+xml")
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			got, err := NewClient(srv.URL, "TESTUSER", "testpass").GetTypeInfo(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("GetTypeInfo: %v", err)
			}
			if gotAccept != "application/vnd.sap.adt.dataelements.v2+xml" {
				t.Errorf("Accept = %q", gotAccept)
			}
			if gotPath != tt.wantPath {
				t.Errorf("escaped path = %q, want %q", gotPath, tt.wantPath)
			}
			if *got != tt.want {
				t.Errorf("TypeInfo = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestGetTypeInfoRejectsErrorAndInvalidDocuments(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantError  string
	}{
		{name: "not found", status: http.StatusNotFound, body: `<exc:exception>not found</exc:exception>`, wantStatus: http.StatusNotFound},
		{name: "unsupported representation", status: http.StatusNotAcceptable, body: `<exc:exception>not acceptable</exc:exception>`, wantStatus: http.StatusNotAcceptable},
		{name: "malformed xml", status: http.StatusOK, body: `<blue:wbobj><dtel:dataElement>`, wantError: "parsing data element document"},
		{name: "html login page", status: http.StatusOK, body: `<!doctype html><html><body>Sign in</body></html>`, wantError: `root element is "html"`},
		{name: "missing element payload", status: http.StatusOK, body: `<blue:wbobj xmlns:blue="urn:wbobj" xmlns:adtcore="urn:core" adtcore:name="ZDEMO_MISSING"/>`, wantError: "missing dataElement"},
		{name: "missing length is not a zero", status: http.StatusOK, body: `<blue:wbobj xmlns:blue="urn:wbobj" xmlns:adtcore="urn:core" xmlns:dtel="urn:dtel" adtcore:name="ZDEMO_MISSING"><dtel:dataElement><dtel:dataType>CHAR</dtel:dataType><dtel:dataTypeDecimals>0</dtel:dataTypeDecimals></dtel:dataElement></blue:wbobj>`, wantError: "missing dataTypeLength"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := NewClient(srv.URL, "TESTUSER", "testpass").GetTypeInfo(context.Background(), "ZDEMO_TYPE")
			if err == nil {
				t.Fatal("GetTypeInfo unexpectedly succeeded")
			}
			if tt.wantStatus != 0 {
				var apiErr *APIError
				if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.wantStatus {
					t.Fatalf("error = %v, want API error status %d", err, tt.wantStatus)
				}
			}
			if tt.wantError != "" && !strings.Contains(err.Error(), tt.wantError) {
				t.Errorf("error = %q, want substring %q", err, tt.wantError)
			}
		})
	}
}
