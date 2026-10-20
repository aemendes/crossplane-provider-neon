// Package apis contains Kubernetes API for the Neon provider.
package apis

import (
	"k8s.io/apimachinery/pkg/runtime"

	databasev1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/database/v1alpha1"
	projectv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
	rolev1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/role/v1alpha1"
	neonv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/v1alpha1"
)

// AddToSchemes may be used to add all resources defined in the project to a Scheme.
var AddToSchemes runtime.SchemeBuilder = runtime.SchemeBuilder{
	neonv1alpha1.SchemeBuilder.AddToScheme,
	projectv1alpha1.SchemeBuilder.AddToScheme,
	databasev1alpha1.SchemeBuilder.AddToScheme,
	rolev1alpha1.SchemeBuilder.AddToScheme,
}

// AddToScheme adds all Resources to the Scheme.
func AddToScheme(s *runtime.Scheme) error {
	return AddToSchemes.AddToScheme(s)
}
