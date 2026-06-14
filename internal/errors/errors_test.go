package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewUserFacingError(t *testing.T) {
	err := NewUserFacingError(ErrProviderAuth, "API Key 验证失败", "请检查 API Key")
	if err.Code != ErrProviderAuth {
		t.Errorf("expected code %s, got %s", ErrProviderAuth, err.Code)
	}
	if err.Message != "API Key 验证失败" {
		t.Errorf("unexpected message: %s", err.Message)
	}
	if err.Hint != "请检查 API Key" {
		t.Errorf("unexpected hint: %s", err.Hint)
	}
	if err.Error() != "[ERR_PROVIDER_AUTH] API Key 验证失败" {
		t.Errorf("unexpected error string: %s", err.Error())
	}
}

func TestWrapError(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	err := WrapError(ErrNetworkUnreachable, "无法连接服务器", "检查网络连接", inner)

	if !errors.Is(err, inner) {
		t.Error("Unwrap should return inner error")
	}

	ufe, ok := AsUserFacingError(err)
	if !ok {
		t.Fatal("should be UserFacingError")
	}
	if ufe.Code != ErrNetworkUnreachable {
		t.Errorf("wrong code: %s", ufe.Code)
	}
}

func TestIsUserFacingError(t *testing.T) {
	ufe := NewUserFacingError(ErrInternalError, "test", "")
	if !IsUserFacingError(ufe) {
		t.Error("should be user facing")
	}
	if IsUserFacingError(fmt.Errorf("plain error")) {
		t.Error("should not be user facing")
	}
}

func TestDegradationMatrix(t *testing.T) {
	m := NewDegradationMatrix()
	if m.IsDegraded() {
		t.Error("new matrix should not be degraded")
	}
	if m.GetOverallLevel() != DegradationNone {
		t.Error("new matrix level should be None")
	}

	m.ReportDegradation("indexer", &DegradationStatus{
		Level:  DegradationMinor,
		Reason: "tree-sitter CGO 不可用",
	})
	if !m.IsDegraded() {
		t.Error("should be degraded after report")
	}
	if m.GetOverallLevel() != DegradationMinor {
		t.Error("overall level should be Minor")
	}

	m.ReportDegradation("tui", &DegradationStatus{
		Level:  DegradationMajor,
		Reason: "终端不支持 ANSI",
	})
	if m.GetOverallLevel() != DegradationMajor {
		t.Error("overall level should be Major (highest)")
	}

	summary := m.Summary()
	if summary == "全功能运行中" {
		t.Error("summary should show degradation")
	}
}
