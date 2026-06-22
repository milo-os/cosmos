package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
)

// BGPPeerState represents the BGP Finite State Machine state of a session.
type BGPPeerState string

const (
	BGPPeerStateIdle        BGPPeerState = "Idle"
	BGPPeerStateConnect     BGPPeerState = "Connect"
	BGPPeerStateActive      BGPPeerState = "Active"
	BGPPeerStateOpenSent    BGPPeerState = "OpenSent"
	BGPPeerStateOpenConfirm BGPPeerState = "OpenConfirm"
	BGPPeerStateEstablished BGPPeerState = "Established"
)

// Condition type constants for BGPPeer.
//
// These follow Kubernetes condition conventions: a small set of stable,
// semantically-meaningful types whose Status/Reason/Message fields carry
// the dynamic detail (via Reason = FSM state, Message = explanation).
const (
	// ConditionTypeReady indicates whether the BGP session is fully up.
	// True when sessionState == Established; False otherwise with Reason
	// set to the current FSM state (e.g. "OpenSent", "Active").
	ConditionTypeReady string = "Ready"

	// ConditionTypeAccepted indicates whether the peer configuration has
	// been accepted by the BGP runtime (FRR/GoBGP, etc.).
	ConditionTypeAccepted string = "Accepted"
)

// Idle sub-reasons — used as Ready.Reason when sessionState == Idle.
const (
	// IdleReasonBackOff indicates exponential back-off before next
	// connection attempt.
	IdleReasonBackOff string = "BackOff"

	// IdleReasonConnectionRefused indicates the TCP connection was
	// actively refused by the peer.
	IdleReasonConnectionRefused string = "ConnectionRefused"

	// IdleReasonHoldTimerExpired indicates the peer failed to send
	// KEEPALIVE within the hold timer.
	IdleReasonHoldTimerExpired string = "HoldTimerExpired"

	// IdleReasonIdle is the default when the session is simply not
	// started (no recent failure).
	IdleReasonIdle string = "Idle"
)

// BGPPeer defines a BGP session to a remote peer. It binds to one or more
// BGPRouter instances via routerRef or routerSelector.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=bgppr
// +kubebuilder:printcolumn:name="ROUTER",type="string",JSONPath=".spec.routerRef.name"
// +kubebuilder:printcolumn:name="PEER-ADDRESS",type="string",JSONPath=".spec.address"
// +kubebuilder:printcolumn:name="PEER-ASN",type="integer",JSONPath=".spec.peerASN"
// +kubebuilder:printcolumn:name="STATE",type="string",JSONPath=".status.sessionState"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
type BGPPeer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BGPPeerSpec   `json:"spec,omitempty"`
	Status BGPPeerStatus `json:"status,omitempty"`
}

// BGPPeerSpec defines the desired state of BGPPeer.
type BGPPeerSpec struct {
	RouterTarget `json:",inline"`

	// PeerASN is the remote AS number.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	PeerASN int64 `json:"peerASN"`

	// Address is the remote peer's IPv4 or IPv6 address.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="isIP(self)",message="address must be a valid IPv4 or IPv6 address"
	Address string `json:"address"`

	// Description is a human-readable label for this peer (e.g., "spine-1").
	// +optional
	Description string `json:"description,omitempty"`

	// AuthSecretRef references a Secret in the same namespace containing the
	// MD5 TCP authentication password under the key "password".
	// +optional
	AuthSecretRef *LocalSecretRef `json:"authSecretRef,omitempty"`

	// AddressFamilies defines the address families negotiated on this session.
	// +kubebuilder:validation:MinItems=1
	AddressFamilies []AddressFamily `json:"addressFamilies"`

	// HoldTime is the BGP hold timer. Must be 0 (disabled) or >= 3s.
	// Defaults to 90s if unset.
	// +optional
	// +kubebuilder:validation:XValidation:rule="duration(self) == duration('0s') || duration(self) >= duration('3s')",message="holdTime must be 0 or >= 3s"
	HoldTime *metav1.Duration `json:"holdTime,omitempty"`

	// KeepaliveTime is the BGP keepalive interval. Must be <= HoldTime / 3.
	// Defaults to 30s if unset.
	// +optional
	KeepaliveTime *metav1.Duration `json:"keepaliveTime,omitempty"`
}

// BGPPeerStatus defines the observed state of BGPPeer.
type BGPPeerStatus struct {
	// ObservedGeneration is the .metadata.generation this status was computed from.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// SessionState is the current BGP FSM state of this session.
	// +optional
	SessionState BGPPeerState `json:"sessionState,omitempty"`

	// LastEstablishedTime is the timestamp of the most recent Established transition.
	// +optional
	LastEstablishedTime *metav1.Time `json:"lastEstablishedTime,omitempty"`

	// Conditions contains the standard conditions for this resource.
	//
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type BGPPeerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BGPPeer `json:"items"`
}

// updatePeerConditions updates the Ready condition based on the current
// BGP FSM state. It is a reference implementation for controller authors.
//
// The method mutates and returns the conditions slice in-place via
// metav1.SetStatusCondition, following Kubernetes condition conventions:
//   - Ready.Status = True only when session is Established
//   - Ready.Status = False for all intermediate states, with Reason set
//     to the FSM state string (e.g. "OpenSent", "Active")
//   - For Idle, Reason is delegated to the caller via the idleReason arg
//
// The ObservedGeneration is set to gen so consumers can correlate this
// status with the spec generation that produced it.
func (s *BGPPeerStatus) updatePeerConditions(state BGPPeerState, gen int64, idleReason string) {
	ready := metav1.Condition{
		Type:               ConditionTypeReady,
		ObservedGeneration: gen,
	}

	switch state {
	case BGPPeerStateEstablished:
		ready.Status = metav1.ConditionTrue
		ready.Reason = "Established"
		ready.Message = "BGP session is Established; address families negotiated."
	case BGPPeerStateOpenConfirm:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "OpenConfirm"
		ready.Message = "BGP session in OpenConfirm state, awaiting KEEPALIVE."
	case BGPPeerStateOpenSent:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "OpenSent"
		ready.Message = "BGP OPEN message sent, awaiting peer OPEN."
	case BGPPeerStateActive:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "Active"
		ready.Message = "BGP session Active, attempting to establish TCP connection."
	case BGPPeerStateConnect:
		ready.Status = metav1.ConditionFalse
		ready.Reason = "Connect"
		ready.Message = "BGP session in Connect state, waiting for TCP connection."
	case BGPPeerStateIdle:
		ready.Status = metav1.ConditionFalse
		ready.Reason = idleReason
		ready.Message = "BGP session is Idle."
	default:
		// Unknown state — treat as idle with a generic reason.
		ready.Status = metav1.ConditionFalse
		ready.Reason = "Unknown"
		ready.Message = fmt.Sprintf("BGP session is in unknown state %q.", state)
	}

	meta.SetStatusCondition(&s.Conditions, ready)
}

// SetAcceptedCondition updates (or creates) the Accepted condition.
// Call this after validating and accepting the peer spec into the runtime.
func (s *BGPPeerStatus) SetAcceptedCondition(accepted bool, gen int64, reason, message string) {
	status := metav1.ConditionFalse
	if accepted {
		status = metav1.ConditionTrue
	}
	cond := metav1.Condition{
		Type:               ConditionTypeAccepted,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: gen,
	}
	meta.SetStatusCondition(&s.Conditions, cond)
}

func init() {
	SchemeBuilder.Register(&BGPPeer{}, &BGPPeerList{})
}
