package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// The dataelements endpoint refuses application/xml with 406 on every name, so
// both callers of it must send the versioned vocabulary type. GetDataElementLabels
// was fixed for this once; GetTypeInfo was missed and kept the generic type, which
// meant it had never returned anything to anybody. These two tests pin the header
// on BOTH callers, because the bug was a twin drifting out of step with its twin —
// pinning only the one that broke would let the next divergence through.
const dataElementsAcceptV2 = "application/vnd.sap.adt.dataelements.v2+xml"

func TestGetTypeInfo_SendsVersionedAccept(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?><blue:wbobj adtcore:name="APC_CONNECTION_ID" adtcore:type="DTEL/DE" adtcore:description="Connection ID" xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel" xmlns:adtcore="http://www.sap.com/adt/core"><dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements"><dtel:typeKind>domain</dtel:typeKind></dtel:dataElement></blue:wbobj>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/dataelements/APC_CONNECTION_ID": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.GetTypeInfo(context.Background(), "APC_CONNECTION_ID"); err != nil {
		t.Fatalf("GetTypeInfo failed: %v", err)
	}

	assertDataElementsAccept(t, mock, "GetTypeInfo")
}

func TestGetDataElementLabels_SendsVersionedAccept(t *testing.T) {
	xmlResp := `<?xml version="1.0" encoding="utf-8"?><blue:wbobj xmlns:blue="http://www.sap.com/wbobj/dictionary/dtel"><dtel:dataElement xmlns:dtel="http://www.sap.com/adt/dictionary/dataelements"><dtel:shortFieldLabel>X</dtel:shortFieldLabel></dtel:dataElement></blue:wbobj>`

	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"/sap/bc/adt/ddic/dataelements/ZDEMO_ORDER_ID": newTestResponse(xmlResp),
			"discovery": newTestResponse("OK"),
		},
	}

	cfg := NewConfig("https://sap.example.com:44300", "user", "pass")
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	if _, err := client.GetDataElementLabels(context.Background(), "ZDEMO_ORDER_ID", "EN"); err != nil {
		t.Fatalf("GetDataElementLabels failed: %v", err)
	}

	assertDataElementsAccept(t, mock, "GetDataElementLabels")
}

// assertDataElementsAccept checks the request that actually went to the
// dataelements endpoint, not merely the last request made, so a CSRF or
// discovery hop in between cannot make the assertion pass by accident.
func assertDataElementsAccept(t *testing.T, mock *mockTransportClient, caller string) {
	t.Helper()

	found := false
	for _, req := range mock.requests {
		if !strings.Contains(req.URL.Path, "/sap/bc/adt/ddic/dataelements/") {
			continue
		}
		found = true
		if got := req.Header.Get("Accept"); got != dataElementsAcceptV2 {
			t.Errorf("%s sent Accept %q to the dataelements endpoint, want %q\n"+
				"A generic type is refused there with 406 on every name, so this "+
				"call would return nothing to anybody.", caller, got, dataElementsAcceptV2)
		}
	}
	if !found {
		t.Fatalf("%s made no request to the dataelements endpoint", caller)
	}
}
