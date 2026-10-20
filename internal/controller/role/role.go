// Package role manages the lifecycle of Neon Role managed resources.
package role

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	crossplanecontroller "github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/role/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

const (
	errNotRole      = "managed resource is not a Role custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errResolveBranch = "cannot resolve branch"
	errObserve       = "cannot observe Neon role"
	errCreate        = "cannot create Neon role"
	errDelete        = "cannot delete Neon role"
)

// Setup adds a controller that reconciles Role managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.RoleKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.RoleKind)),
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newClientFn: neon.NewClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...),
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.Role{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the Neon API key out of
// the ProviderConfig referenced by the Role being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiKey string) *neon.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Role)
	if !ok {
		return nil, errors.New(errNotRole)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackPCUsage)
	}

	pc := &apisv1alpha1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetPC)
	}

	data, err := resource.CommonCredentialExtractor(ctx, pc.Spec.Credentials.Source, c.kube, pc.Spec.Credentials.CommonCredentialSelectors)
	if err != nil {
		return nil, errors.Wrap(err, errGetCreds)
	}

	return &external{client: c.newClientFn(string(data))}, nil
}

// external observes, creates and deletes Neon roles on behalf of a Role managed resource.
type external struct {
	client *neon.Client
}

// Compile-time checks that connector/external satisfy the crossplane-runtime
// interfaces managed.NewReconciler requires. A missing method here surfaces
// only as a runtime panic on provider startup.
var (
	_ managed.ExternalConnecter = &connector{}
	_ managed.ExternalClient    = &external{}
)

func (e *external) Disconnect(_ context.Context) error {
	return nil
}

// resolveBranchID returns cr's configured branch, or the project's default
// branch if none was configured.
func (e *external) resolveBranchID(ctx context.Context, cr *v1alpha1.Role) (string, error) {
	if cr.Spec.ForProvider.BranchID != "" {
		return cr.Spec.ForProvider.BranchID, nil
	}
	b, err := e.client.DefaultBranch(ctx, cr.Spec.ForProvider.ProjectID)
	if err != nil {
		return "", err
	}
	return b.ID, nil
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr, ok := mg.(*v1alpha1.Role)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotRole)
	}

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		if neon.IsNotFound(err) {
			// The parent project (or its default branch) is gone, so the role
			// can't exist either. Report it as gone rather than erroring forever,
			// otherwise Delete is never reached and the finalizer never clears.
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errResolveBranch)
	}

	r, err := e.client.GetRole(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name)
	if err != nil {
		if neon.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.RoleObservation{
		BranchID:  branchID,
		Protected: r.Protected,
		CreatedAt: r.CreatedAt,
	}

	// Roles have no mutable fields via Neon's API (no PATCH endpoint exists), so
	// once a role exists it's always up to date — Update is never called.
	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: true,
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Role)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotRole)
	}

	cr.SetConditions(xpv1.Creating())

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errResolveBranch)
	}

	r, err := e.client.CreateRole(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	conn := managed.ConnectionDetails{
		"username": []byte(r.Name),
	}
	if r.Password != "" {
		conn["password"] = []byte(r.Password)
	}

	return managed.ExternalCreation{ConnectionDetails: conn}, nil
}

// Update is a no-op: Observe always reports ResourceUpToDate=true, since Neon
// has no PATCH endpoint for roles, so the reconciler never calls this.
func (e *external) Update(_ context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	if _, ok := mg.(*v1alpha1.Role); !ok {
		return managed.ExternalUpdate{}, errors.New(errNotRole)
	}
	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Role)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotRole)
	}

	cr.SetConditions(xpv1.Deleting())

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		if neon.IsNotFound(err) {
			// The parent project (or its default branch) is already gone, so
			// there's nothing left to delete.
			return managed.ExternalDelete{}, nil
		}
		return managed.ExternalDelete{}, errors.Wrap(err, errResolveBranch)
	}

	if err := e.client.DeleteRole(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}
