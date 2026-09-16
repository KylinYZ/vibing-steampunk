package adt

import (
	"context"
	"net/http"
	"testing"
)

func TestFeatureProbeUsesDiscoveryInsteadOfOptions(t *testing.T) {
	discovery := `<app:service><app:collection href="/sap/bc/adt/ddic/ddl/sources"/><app:collection href="/sap/bc/adt/cts/transportrequests"/></app:service>`
	for _, id := range []FeatureID{FeatureRAP, FeatureTransport} {
		mock := &mockTransportClient{responses: map[string]*http.Response{
			"/sap/bc/adt/discovery": newTestResponse(discovery),
		}}
		cfg := NewConfig("https://sap.example.com", "user", "pass")
		client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))
		prober := NewFeatureProber(client, DefaultFeatureConfig(), false)
		status := prober.Probe(context.Background(), id)
		if !status.Available {
			t.Errorf("%s should be available from discovery: %s", id, status.Message)
		}
		for _, request := range mock.requests {
			if request.Method == http.MethodOptions {
				t.Fatalf("feature probe must not use OPTIONS: %s", request.URL)
			}
		}
	}
}
