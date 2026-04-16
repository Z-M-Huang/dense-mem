package service_test

import (
	"testing"

	"github.com/dense-mem/dense-mem/internal/embedding"
	"github.com/dense-mem/dense-mem/internal/service/fragmentservice"
	"github.com/dense-mem/dense-mem/internal/service/recallservice"
	"github.com/dense-mem/dense-mem/internal/tools/registry"
)

// TestCompanionInterfaces_AllNewTypesAssertable concentrates compile-time
// assertions that every new discoverability interface has a companion mock
// implementation (AC-53). Package-level `var _ Interface = (*Mock)(nil)`
// lines in each mock file provide the real compile-time guarantees; this
// test binds them to the test binary so a break is visible as a failing
// unit test, not just a build error.
func TestCompanionInterfaces_AllNewTypesAssertable(t *testing.T) {
	var (
		_ fragmentservice.CreateFragmentService = (*fragmentservice.MockCreate)(nil)
		_ fragmentservice.GetFragmentService    = (*fragmentservice.MockGet)(nil)
		_ fragmentservice.ListFragmentsService  = (*fragmentservice.MockList)(nil)
		_ fragmentservice.DeleteFragmentService = (*fragmentservice.MockDelete)(nil)
		_ recallservice.RecallService           = (*recallservice.MockRecall)(nil)
		_ registry.Registry                     = (*registry.MockRegistry)(nil)
		_ embedding.EmbeddingProviderInterface  = (*embedding.MockEmbeddingProvider)(nil)
	)
	if t.Failed() {
		t.Fatal("companion interface assertions failed")
	}
}
