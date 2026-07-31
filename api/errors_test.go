package api

import (
	"encoding/json"
	"testing"
)

func TestErrorResponseJSON(t *testing.T) {
	value := ErrorResponse{Error: Error{
		Code: ErrorCodeValidationFailed, Message: "request validation failed", RequestID: "req_test",
		Details: []ErrorDetail{{Field: "model_ref", Reason: "is required"}},
	}}
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"error":{"code":"validation_failed","message":"request validation failed","request_id":"req_test","details":[{"field":"model_ref","reason":"is required"}]}}`
	if string(encoded) != want {
		t.Fatalf("error response = %s, want %s", encoded, want)
	}
}
