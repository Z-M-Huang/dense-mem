package service

import (
	"context"
	"testing"

	"github.com/dense-mem/dense-mem/internal/requestctx"
)

func TestAuditClientIPValueUsesEntryClientIP(t *testing.T) {
	ctx := requestctx.WithClientIP(context.Background(), "192.168.1.101")
	got := auditClientIPValue(ctx, AuditLogEntry{ClientIP: "203.0.113.10"})
	if got != "203.0.113.10" {
		t.Errorf("auditClientIPValue() = %#v; want 203.0.113.10", got)
	}
}

func TestAuditClientIPValueFallsBackToContextClientIP(t *testing.T) {
	ctx := requestctx.WithClientIP(context.Background(), "192.168.1.101")
	got := auditClientIPValue(ctx, AuditLogEntry{})
	if got != "192.168.1.101" {
		t.Errorf("auditClientIPValue() = %#v; want 192.168.1.101", got)
	}
}

func TestAuditClientIPValueReturnsNilWhenMissing(t *testing.T) {
	got := auditClientIPValue(context.Background(), AuditLogEntry{ClientIP: "   "})
	if got != nil {
		t.Errorf("auditClientIPValue() = %#v; want nil", got)
	}
}
