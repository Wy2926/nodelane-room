package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPublicCodeLocalizationAndSafeDetails(t *testing.T) {
	for _, locale := range []string{"zh-CN", "en-US"} {
		raw, err := os.ReadFile(filepath.Join("..", "..", "desktop", "src", "i18n", "locales", locale+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var messages map[string]string
		if err = json.Unmarshal(raw, &messages); err != nil {
			t.Fatal(err)
		}
		for code, spec := range BusinessCodes {
			if messages["business."+code] == "" || spec.Message == "" {
				t.Error("missing public code localization", locale, code)
			}
		}
	}
	secret := Details{RoomID: "unrelated-room", DeviceID: "unrelated-device", Fields: []FieldIssue{{Field: "private_key", Rule: "invalid_format"}, {Field: "name", Rule: "too_long"}}}
	value := SafeDetails("request_validation_failed", secret)
	if value.RoomID != "" || value.DeviceID != "" || len(value.Fields) != 1 || value.Fields[0].Field != "name" {
		t.Fatal("details whitelist failed")
	}
	if EndsIdentity("room_owner_required") || EndsIdentity("account_disabled") || EndsMembership("account_disabled") || EndsMembership("resource_not_found") {
		t.Fatal("generic failure ended authorization")
	}
}
