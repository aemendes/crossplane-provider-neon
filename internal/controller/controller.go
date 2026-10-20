// Package controller aggregates the controllers for this provider.
package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	xpcontroller "github.com/crossplane/crossplane-runtime/pkg/controller"

	"github.com/aemendes/crossplane-provider-neon/internal/controller/database"
	"github.com/aemendes/crossplane-provider-neon/internal/controller/project"
	"github.com/aemendes/crossplane-provider-neon/internal/controller/providerconfig"
	"github.com/aemendes/crossplane-provider-neon/internal/controller/role"
)

// Setup creates all controllers with the supplied logger and adds them to the supplied manager.
func Setup(mgr ctrl.Manager, o xpcontroller.Options) error {
	for _, setup := range []func(ctrl.Manager, xpcontroller.Options) error{
		providerconfig.Setup,
		project.Setup,
		database.Setup,
		role.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}
