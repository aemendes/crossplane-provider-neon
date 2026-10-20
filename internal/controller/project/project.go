// Package project manages the lifecycle of Neon Project managed resources.
package project

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
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"

	v1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/project/v1alpha1"
	apisv1alpha1 "github.com/aemendes/crossplane-provider-neon/apis/v1alpha1"
	"github.com/aemendes/crossplane-provider-neon/internal/clients/neon"
)

const (
	errNotProject   = "managed resource is not a Project custom resource"
	errTrackPCUsage = "cannot track ProviderConfig usage"
	errGetPC        = "cannot get ProviderConfig"
	errGetCreds     = "cannot get credentials"

	errObserve = "cannot observe Neon project"
	errCreate  = "cannot create Neon project"
	errUpdate  = "cannot update Neon project"
	errDelete  = "cannot delete Neon project"
)

// Setup adds a controller that reconciles Project managed resources.
func Setup(mgr ctrl.Manager, o crossplanecontroller.Options) error {
	name := managed.ControllerName(v1alpha1.ProjectKind)

	cps := []managed.ConnectionPublisher{managed.NewAPISecretPublisher(mgr.GetClient(), mgr.GetScheme())}

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ProjectKind)),
		managed.WithExternalConnecter(&connector{
			kube:        mgr.GetClient(),
			usage:       resource.NewProviderConfigUsageTracker(mgr.GetClient(), &apisv1alpha1.ProviderConfigUsage{}),
			newClientFn: neon.NewClient,
		}),
		// Disable the reconciler's default initializer, which otherwise sets
		// external-name to the managed resource's own metadata.name before the
		// first Observe ever runs. That would make Observe's adopt-by-name
		// branch below (which only fires when external-name is empty)
		// unreachable, defeating its purpose of avoiding duplicate, billable
		// Neon projects if a prior Create's response was lost.
		managed.WithInitializers(),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithConnectionPublishers(cps...),
		managed.WithPollInterval(o.PollInterval),
		managed.WithManagementPolicies(),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(controller.Options{MaxConcurrentReconciles: o.MaxConcurrentReconciles}).
		For(&v1alpha1.Project{}).
		Complete(r)
}

// connector produces an ExternalClient by reading the Neon API key out of
// the ProviderConfig referenced by the Project being reconciled.
type connector struct {
	kube        client.Client
	usage       resource.Tracker
	newClientFn func(apiKey string) *neon.Client
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return nil, errors.New(errNotProject)
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

// external observes, creates, updates and deletes Neon projects on behalf of a Project managed resource.
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

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) { //nolint:dupl // Observe/Create/Update share a similar shape but diverge in behaviour.
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalObservation{}, errors.New(errNotProject)
	}

	id := meta.GetExternalName(cr)
	if id == "" {
		// The external-name annotation is unset. Before declaring the resource
		// doesn't exist, check whether a project with this exact name already
		// exists in Neon: if a prior Create succeeded but the pod died before
		// persisting the annotation, blindly creating again would leave a
		// duplicate, billable Neon project behind (Neon has no idempotency key).
		found, err := e.findByExactName(ctx, desiredName(cr))
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
		}
		if found == nil {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		meta.SetExternalName(cr, found.ID)
		obs := e.observed(cr, found)
		// Observe's annotation changes aren't persisted unless we say so — flag
		// this one as a late-init so the reconciler writes the adopted
		// external-name back to the API server instead of re-discovering it via
		// a ListProjectsByName call on every subsequent poll.
		obs.ResourceLateInitialized = true
		return obs, nil
	}

	p, err := e.client.GetProject(ctx, id)
	if err != nil {
		if neon.IsNotFound(err) {
			return managed.ExternalObservation{ResourceExists: false}, nil
		}
		return managed.ExternalObservation{}, errors.Wrap(err, errObserve)
	}

	return e.observed(cr, p), nil
}

// observed records the given external state on cr and returns the resulting
// ExternalObservation. p is assumed to exist (a successful Get, or a Create/List match).
func (e *external) observed(cr *v1alpha1.Project, p *neon.Project) managed.ExternalObservation {
	cr.SetConditions(xpv1.Available())
	cr.Status.AtProvider = v1alpha1.ProjectObservation{
		ID:        p.ID,
		Host:      p.ProxyHost,
		RegionID:  p.RegionID,
		PGVersion: p.PgVersion,
		CreatedAt: p.CreatedAt,
	}

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: p.Name == desiredName(cr),
	}
}

// findByExactName searches Neon for a project with exactly the given name,
// returning nil if none is found. Neon's search endpoint only does partial
// matching, so results are filtered client-side for an exact match.
func (e *external) findByExactName(ctx context.Context, name string) (*neon.Project, error) {
	candidates, err := e.client.ListProjectsByName(ctx, name)
	if err != nil {
		return nil, err
	}
	for i := range candidates {
		if candidates[i].Name == name {
			return &candidates[i], nil
		}
	}
	return nil, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalCreation{}, errors.New(errNotProject)
	}

	cr.SetConditions(xpv1.Creating())

	resp, err := e.client.CreateProject(ctx, desiredName(cr), cr.Spec.ForProvider.RegionID, cr.Spec.ForProvider.PGVersion)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreate)
	}

	meta.SetExternalName(cr, resp.Project.ID)

	conn := managed.ConnectionDetails{}
	if len(resp.ConnectionURIs) > 0 {
		conn["connectionUri"] = []byte(resp.ConnectionURIs[0].ConnectionURI)
	}

	return managed.ExternalCreation{ConnectionDetails: conn}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalUpdate{}, errors.New(errNotProject)
	}

	if _, err := e.client.UpdateProject(ctx, meta.GetExternalName(cr), desiredName(cr)); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdate)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) (managed.ExternalDelete, error) {
	cr, ok := mg.(*v1alpha1.Project)
	if !ok {
		return managed.ExternalDelete{}, errors.New(errNotProject)
	}

	cr.SetConditions(xpv1.Deleting())

	if err := e.client.DeleteProject(ctx, meta.GetExternalName(cr)); err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errDelete)
	}

	return managed.ExternalDelete{}, nil
}

// desiredName is the project name to reconcile towards: the user-supplied
// name if set, falling back to the managed resource's own name.
func desiredName(cr *v1alpha1.Project) string {
	if cr.Spec.ForProvider.Name != "" {
		return cr.Spec.ForProvider.Name
	}
	return cr.GetName()
}
