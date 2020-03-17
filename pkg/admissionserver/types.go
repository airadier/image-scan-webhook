package admissionserver

import (
	"image-scan-webhook/pkg/opaimagescanner"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type admissionHook struct {
	evaluator opaimagescanner.AdmissionEvaluator
}

type mutationHook struct {
	evaluator opaimagescanner.AdmissionEvaluator
}

// toAdmissionResponse is a helper function to create an AdmissionResponse
// with an embedded error
func toAdmissionResponse(uid types.UID, err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		UID:     uid,
		Allowed: false,
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}
