// Package database manages the lifecycle of Neon Database managed resources.
package database

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

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/database/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

const (
	errNotDatabase  = "managed resource is not a Database custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errResolveBranch = "cannot resolve branch"
	errObserve       = "cannot observe Neon database"
	errCreate        = "cannot create Neon database"
	errUpdate        = "cannot update Neon database"
	errDelete        = "cannot delete Neon database"
)

// Setup adds a controller that reconciles Database managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.DatabaseKind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.DatabaseKind)),
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newClientFn: neon.NewClient,
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.Database{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the Neon API key out of
// the ProviderConfig referenced by the Database being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiKey string) *neon.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Database)
	if !ok {
		return nil, errors.New(errNotDatabase)
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

// external observes, creates, updates and deletes Neon databases on behalf of a Database managed resource.
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
func (e *external) resolveBranchID(ctx context.Context, cr *v1alpha1.Database) (string, error) {
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
	cr, ok := mg.(*v1alpha1.Database)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotDatabase)
	}

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		if neon.IsNotFound(err) {
			// The parent project (or its default branch) is gone, so the database
			// can't exist either. Report it as gone rather than erroring forever,
			// otherwise Delete is never reached and the finalizer never clears.
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errResolveBranch)
	}

	db, err := e.client.GetDatabase(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name)
	if err != nil {
		if neon.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.DatabaseObservation{
		BranchID:  branchID,
		OwnerName: db.OwnerName,
		CreatedAt: db.CreatedAt,
	}

	// If the database was created without an explicit owner, Neon assigns one
	// (typically the project's default role) rather than leaving it empty.
	// Adopt that as spec.forProvider.ownerName instead of leaving the spec
	// blank forever — otherwise the owner-name comparison below never matches
	// and Update fires a no-op PATCH on every poll.
	lateInit := false
	if cr.Spec.ForProvider.OwnerName == "" && db.OwnerName != "" {
		cr.Spec.ForProvider.OwnerName = db.OwnerName
		lateInit = true
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        db.OwnerName == cr.Spec.ForProvider.OwnerName,
		ResourceLateInitialized: lateInit,
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Database)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotDatabase)
	}

	cr.SetConditions(xpv1.Creating())

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errResolveBranch)
	}

	if _, err := e.client.CreateDatabase(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name, cr.Spec.ForProvider.OwnerName); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Database)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotDatabase)
	}

	branchID, err := e.resolveBranchID(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errResolveBranch)
	}

	if _, err := e.client.UpdateDatabase(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name, cr.Spec.ForProvider.OwnerName); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Database)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotDatabase)
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

	if err := e.client.DeleteDatabase(ctx, cr.Spec.ForProvider.ProjectID, branchID, cr.Spec.ForProvider.Name); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}
