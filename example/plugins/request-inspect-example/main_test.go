package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	plugin "example.com/zoraxy/request-inspect-example/mod/zoraxy_plugin"
)

func TestInspectQuery(t *testing.T) {
	for _, tc := range []struct{ uri, action string }{
		{"/?block=1", "deny"}, {"/?block=0", "allow"}, {"/", "allow"},
	} {
		t.Run(tc.uri, func(t *testing.T) {
			data, _ := json.Marshal(plugin.RequestInspectPayload{DynamicSniffForwardRequest: plugin.DynamicSniffForwardRequest{Method: "GET", RequestURI: tc.uri}})
			w := httptest.NewRecorder()
			inspect(w, httptest.NewRequest("POST", "/inspect", bytes.NewReader(data)))
			var result plugin.RequestInspectResult
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || result.Action != tc.action {
				t.Fatalf("got %d %+v", w.Code, result)
			}
		})
	}
}
