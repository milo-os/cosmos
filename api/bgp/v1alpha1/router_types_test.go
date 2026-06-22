package v1alpha1

import (
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestRouter(roles ...RouterRole) *BGPRouter {
	return &BGPRouter{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "bgp.miloapis.com/v1alpha1",
			Kind:       "BGPRouter",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "test-router"},
		Spec: BGPRouterSpec{
			TargetRef: TargetRef{Kind: "Node", Name: "node-1"},
			LocalASN:  65000,
			RouterID:  "10.0.0.1",
			Roles:     roles,
		},
	}
}

// TestBGPRouterDeepCopyRoles verifies that mutating Roles on the copy does not
// affect the original.
func TestBGPRouterDeepCopyRoles(t *testing.T) {
	orig := newTestRouter(RouterRoleTransit, RouterRoleFabric)
	dup := orig.DeepCopy()

	dup.Spec.Roles[0] = RouterRoleTenant

	if orig.Spec.Roles[0] != RouterRoleTransit {
		t.Errorf("Roles[0] mutated: got %q, want %q", orig.Spec.Roles[0], RouterRoleTransit)
	}
	if orig.Spec.Roles[1] != RouterRoleFabric {
		t.Errorf("Roles[1] mutated: got %q, want %q", orig.Spec.Roles[1], RouterRoleFabric)
	}
}

// TestBGPRouterDeepCopyNilRoles verifies that a router with no roles deep-copies
// without allocating a non-nil slice.
func TestBGPRouterDeepCopyNilRoles(t *testing.T) {
	orig := newTestRouter()
	dup := orig.DeepCopy()

	if dup.Spec.Roles != nil {
		t.Errorf("expected nil Roles on copy, got %v", dup.Spec.Roles)
	}
}

// TestBGPRouterJSONRoundTripRoles verifies that Roles survives JSON marshal/unmarshal.
func TestBGPRouterJSONRoundTripRoles(t *testing.T) {
	cases := []struct {
		name  string
		roles []RouterRole
	}{
		{"no roles", nil},
		{"single transit", []RouterRole{RouterRoleTransit}},
		{"all roles", []RouterRole{RouterRoleTransit, RouterRoleFabric, RouterRoleTenant}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := newTestRouter(tc.roles...)

			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var got BGPRouter
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if len(got.Spec.Roles) != len(tc.roles) {
				t.Fatalf("Roles len: got %d, want %d", len(got.Spec.Roles), len(tc.roles))
			}
			for i, r := range tc.roles {
				if got.Spec.Roles[i] != r {
					t.Errorf("Roles[%d]: got %q, want %q", i, got.Spec.Roles[i], r)
				}
			}
		})
	}
}

// TestBGPRouterJSONRolesFieldName verifies the JSON key is "roles".
func TestBGPRouterJSONRolesFieldName(t *testing.T) {
	orig := newTestRouter(RouterRoleFabric)
	data, err := json.Marshal(orig.Spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	raw, ok := m["roles"]
	if !ok {
		t.Fatal("expected JSON key \"roles\" not found")
	}
	roles, ok := raw.([]any)
	if !ok || len(roles) != 1 || roles[0] != "fabric" {
		t.Errorf("unexpected roles value: %v", raw)
	}
}

// TestBGPRouterRolesOmitEmpty verifies that a router with no roles omits the
// "roles" key from JSON output.
func TestBGPRouterRolesOmitEmpty(t *testing.T) {
	orig := newTestRouter()
	data, err := json.Marshal(orig.Spec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if _, ok := m["roles"]; ok {
		t.Error("expected \"roles\" key to be absent when empty")
	}
}
