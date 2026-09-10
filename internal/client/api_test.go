package client

import "testing"

func TestControlURLValidation(t *testing.T) {
	for _, u := range []string{"https://room.example", "http://127.0.0.1:8080", "http://localhost:8080"} {
		if err := ValidateURL(u); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=x", "file:///secret"} {
		if ValidateURL(u) == nil {
			t.Fatalf("accepted %s", u)
		}
	}
}
