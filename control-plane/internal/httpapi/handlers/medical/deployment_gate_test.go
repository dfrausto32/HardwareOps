//go:build medical

package medical_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/parcel/control-plane/internal/httpapi/handlers"
	"github.com/parcel/control-plane/internal/store"
	"github.com/parcel/control-plane/internal/store/memory"
)

// desiredStateBody produces a minimal DesiredStateRequest JSON for a single component.
func desiredStateBody(t *testing.T, artifactID string) []byte {
	t.Helper()
	body, _ := json.Marshal(handlers.DesiredStateRequest{
		Components: map[string]handlers.DesiredComponentRequest{
			"main": {ArtifactID: artifactID, DesiredVersion: "1.0.0"},
		},
	})
	return body
}

// TestDeploymentGate_ClassBBlockedWithoutApproval verifies Class B artifacts
// cannot be deployed to a group unless an approved change record exists.
func TestDeploymentGate_ClassBBlockedWithoutApproval(t *testing.T) {
	mem := memory.New()
	artifact := seedClassBArtifact(t, mem)
	groupID := uuid.NewString()

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ClassB without approval: expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploymentGate_ClassCBlockedWithoutApproval verifies Class C artifacts
// are also gated — Class C is the highest criticality.
func TestDeploymentGate_ClassCBlockedWithoutApproval(t *testing.T) {
	mem := memory.New()
	artifact := seedClassCArtifact(t, mem)
	groupID := uuid.NewString()

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("ClassC without approval: expected 422, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploymentGate_ClassAPassesWithoutChangeRecord confirms Class A artifacts
// (configuration data, no safety risk) are exempt from the approval gate.
func TestDeploymentGate_ClassAPassesWithoutChangeRecord(t *testing.T) {
	mem := memory.New()
	artifact := seedClassAArtifact(t, mem)
	groupID := uuid.NewString()

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("ClassA should bypass gate, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploymentGate_ApprovedChangeRecordAllowsDeploy confirms that an approved
// change record lifts the deployment block for a Class B artifact.
func TestDeploymentGate_ApprovedChangeRecordAllowsDeploy(t *testing.T) {
	mem := memory.New()
	artifact := seedClassBArtifact(t, mem)
	groupID := uuid.NewString()

	_, _ = mem.CreateChangeRecord(store.ChangeRecord{
		RecordID:    "cr-approved",
		ArtifactID:  artifact.ArtifactID,
		SafetyClass: "ClassB",
		Status:      "approved",
	})

	req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("approved record should allow deploy, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploymentGate_BreakGlassSucceeds verifies that ?bypass_change_approval=true
// allows the deployment to proceed even without an approved record.
func TestDeploymentGate_BreakGlassSucceeds(t *testing.T) {
	mem := memory.New()
	artifact := seedClassBArtifact(t, mem)
	groupID := uuid.NewString()

	req := httptest.NewRequest(http.MethodPut, "/?bypass_change_approval=true", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("break-glass should succeed, got %d: %s", w.Code, w.Body.String())
	}
}

// TestDeploymentGate_BreakGlassWritesAuditEvent confirms the break-glass bypass
// is audited with action "change_record.bypassed" — a regulatory requirement.
func TestDeploymentGate_BreakGlassWritesAuditEvent(t *testing.T) {
	mem := memory.New()
	artifact := seedClassBArtifact(t, mem)
	groupID := uuid.NewString()

	req := httptest.NewRequest(http.MethodPut, "/?bypass_change_approval=true", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
	req = withURLParam(req, "groupId", groupID)
	w := httptest.NewRecorder()
	handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("break-glass should succeed, got %d", w.Code)
	}

	events, err := mem.ListAuditEvents(store.AuditEventFilter{Action: "change_record.bypassed", Limit: 10})
	if err != nil {
		t.Fatalf("list audit events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected change_record.bypassed audit event but found none")
	}
	if events[0].TargetID != artifact.ArtifactID {
		t.Fatalf("bypass audit event targets wrong artifact: want %q, got %q",
			artifact.ArtifactID, events[0].TargetID)
	}
}

// TestDeploymentGate_ClassBPendingApprovalBlocked ensures that a draft or
// pending_approval change record is not sufficient — only "approved" passes.
func TestDeploymentGate_ClassBPendingApprovalBlocked(t *testing.T) {
	for _, status := range []string{"draft", "pending_approval", "rejected"} {
		t.Run(status, func(t *testing.T) {
			mem := memory.New()
			artifact := seedClassBArtifact(t, mem)
			groupID := uuid.NewString()

			_, _ = mem.CreateChangeRecord(store.ChangeRecord{
				RecordID:    "cr-" + status,
				ArtifactID:  artifact.ArtifactID,
				SafetyClass: "ClassB",
				Status:      status,
			})

			req := httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(desiredStateBody(t, artifact.ArtifactID)))
			req = withURLParam(req, "groupId", groupID)
			w := httptest.NewRecorder()
			handlers.PutDesiredStateGroupWithPolicyMedical(silentLogger(), mem, false, handlers.ArtifactSignaturePolicy{}, nil).ServeHTTP(w, req)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status=%q should be blocked (422), got %d: %s", status, w.Code, w.Body.String())
			}
		})
	}
}
