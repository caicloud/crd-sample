package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// LabelPrefix defines the format of label which is affected by this resource
	// Label format "labels.caicloud.io/xxx: yyy"
	LabelPrefix = "labels.caicloud.io/"

	// LabelFinalizer defines finalizer of the label
	LabelFinalizer = "labels.caicloud.io"

	// LabelAnnotationKey defines key of the annotation
	// Label name refers will be stored in the annotation
	// with json array format
	LabelAnnotationKey = "labels.caicloud.io"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LabelCounter represents a labelset of resource.
type LabelCounter struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired identities of label
	// +optional
	Spec LabelCounterSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	// Status defines the current status of label
	// +optional
	Status LabelCounterStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=spec"`
}

// LabelCounterSpec represents label attributes
type LabelCounterSpec struct {
	// Values defines counted label values
	Values []string `json:"values" protobuf:"bytes,1,cap,name=values"`
}

// LabelCounterStatus represents label status
type LabelCounterStatus struct {
	Counters []Counter `json:"counters" protobuf:"bytes,1,cap,name=counters"`
}

// Counter defines the resource number
type Counter struct {
	// Value defines the value of the label
	Value string `json:"value" protobuf:"bytes,1,req,name=value"`

	// Count defines count of the resources
	Count int `json:"count" protobuf:"bytes,2,req,name=count"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LabelCounterList is a collection of labels
type LabelCounterList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items defines an array of application
	Items []LabelCounter `json:"items" protobuf:"bytes,2,rep,name=items"`
}
