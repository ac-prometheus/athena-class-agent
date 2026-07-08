package aegis

import (
	"testing"
)

func TestScanOutbound_CleanContent(t *testing.T) {
	report := ScanOutbound("This is a clean response with no sensitive data.")
	if !report.Clean {
		t.Errorf("expected clean report, got findings: %v", report.Findings)
	}
}

func TestScanOutbound_APIKey(t *testing.T) {
	report := ScanOutbound("Use this key: sk-abc123def456ghi789 to authenticate.")
	if report.Clean {
		t.Error("expected findings for API key prefix sk-")
	}
	found := false
	for _, f := range report.Findings {
		if f == "possible API key or credential: contains sk-" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected sk- finding, got: %v", report.Findings)
	}
}

func TestScanOutbound_BearerToken(t *testing.T) {
	report := ScanOutbound("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9")
	if report.Clean {
		t.Error("expected finding for Bearer token")
	}
}

func TestScanOutbound_UUID(t *testing.T) {
	report := ScanOutbound("The session is 550e8400-e29b-41d4-a716-446655440000.")
	if report.Clean {
		t.Error("expected UUID finding")
	}
	found := false
	for _, f := range report.Findings {
		if f == "UUID pattern detected" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected UUID finding, got: %v", report.Findings)
	}
}

func TestScanOutbound_FilePath(t *testing.T) {
	report := ScanOutbound("Config file is at /home/user/.config/app.yaml")
	if report.Clean {
		t.Error("expected finding for /home/ path prefix")
	}
}

func TestScanOutbound_DSN(t *testing.T) {
	report := ScanOutbound("Connect via postgres://user:pass@localhost/db")
	if report.Clean {
		t.Error("expected DSN finding")
	}
}

func TestScanOutbound_AnthropicKey(t *testing.T) {
	report := ScanOutbound("export ANTHROPIC_API_KEY=sk-ant-abc123")
	if report.Clean {
		t.Error("expected finding for ANTHROPIC_API_KEY")
	}
}
