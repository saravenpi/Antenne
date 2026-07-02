package config

import "testing"

func TestValidateProductionRejectsWeakConfig(t *testing.T) {
	c := Config{
		Env:           "production",
		JWTSecret:     "dev-secret-change-me",
		AdminPassword: "change-me",
		ClientOrigin:  "*",
	}
	if _, err := c.Validate(); err == nil {
		t.Fatal("expected production validation to fail on default secret/password/origin")
	}
}

func TestValidateProductionAcceptsStrongConfig(t *testing.T) {
	c := Config{
		Env:           "production",
		JWTSecret:     "b1946ac92492d2347c6235b4d2611184b1946ac9", // >=16, not a placeholder
		AdminPassword: "a-strong-password",
		ClientOrigin:  "https://radio.example.com",
	}
	warnings, err := c.Validate()
	if err != nil {
		t.Fatalf("expected strong production config to pass, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got: %v", warnings)
	}
}

func TestValidateDevelopmentOnlyWarns(t *testing.T) {
	c := Config{
		Env:           "development",
		JWTSecret:     "dev-secret-change-me",
		AdminPassword: "admin",
		ClientOrigin:  "*",
	}
	warnings, err := c.Validate()
	if err != nil {
		t.Fatalf("development must not fail validation, got: %v", err)
	}
	if len(warnings) == 0 {
		t.Fatal("expected warnings for weak development config")
	}
}

func TestShortSecretIsWeakInProduction(t *testing.T) {
	c := Config{
		Env:           "production",
		JWTSecret:     "tooshort",
		AdminPassword: "a-strong-password",
		ClientOrigin:  "https://radio.example.com",
	}
	if _, err := c.Validate(); err == nil {
		t.Fatal("expected a secret shorter than 16 chars to fail in production")
	}
}
