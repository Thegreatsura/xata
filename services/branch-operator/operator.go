package branchoperator

import (
	"context"

	barmanPluginApi "github.com/cloudnative-pg/plugin-barman-cloud/api/v1"
	"github.com/go-logr/zerologr"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	apiv1 "github.com/xataio/xata-cnpg/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	"xata/internal/envcfg"
	"xata/internal/o11y"
	"xata/internal/service"
	poolv1alpha1 "xata/proto/clusterpool-operator/api/v1alpha1"
	"xata/services/branch-operator/api/v1alpha1"
	"xata/services/branch-operator/pkg/reconciler"
	"xata/services/branch-operator/pkg/wakeup"
)

// Ensure BranchOperatorService implements Service interface
var _ service.Service = (*BranchOperatorService)(nil)

// Ensure BranchOperatorService implements RunnerService interface
var _ service.RunnerService = (*BranchOperatorService)(nil)

type BranchOperatorService struct {
	config  Config
	manager ctrl.Manager
}

// NewBranchOperatorService creates a new instance of the Branch operator service.
func NewBranchOperatorService() *BranchOperatorService {
	return &BranchOperatorService{}
}

func (s *BranchOperatorService) Name() string {
	return "branch-operator"
}

// ReadConfig implements service.Service.
func (s *BranchOperatorService) ReadConfig(ctx context.Context) error {
	if err := envcfg.Read(&s.config); err != nil {
		return err
	}
	return s.config.ParseTolerations()
}

// Init implements service.Service.
func (s *BranchOperatorService) Init(ctx context.Context) error {
	// Get Kubernetes configuration
	config := ctrl.GetConfigOrDie()

	// Create a new scheme and register types
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return err
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := apiv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := barmanPluginApi.AddToScheme(scheme); err != nil {
		return err
	}
	if err := snapshotv1.AddToScheme(scheme); err != nil {
		return err
	}
	if err := poolv1alpha1.AddToScheme(scheme); err != nil {
		return err
	}

	// Create the controller manager
	mgr, err := ctrl.NewManager(config, ctrl.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	// Create and setup the branch reconciler
	reconciler := &reconciler.BranchReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ClustersNamespace: s.config.ClustersNamespace,
		BackupsBucket:     s.config.BackupsBucket,
		BackupsEndpoint:   s.config.BackupsEndpoint,
		Tolerations:       s.config.Tolerations,
		EnforceZone:       s.config.EnforceZone,
		ImagePullSecrets:  s.config.ImagePullSecrets,
	}
	if err := reconciler.SetupWithManager(ctx, mgr); err != nil {
		return err
	}

	// Create and setup the wakeup reconciler
	wakeupReconciler := &wakeup.WakeupReconciler{
		Client:                  mgr.GetClient(),
		Scheme:                  mgr.GetScheme(),
		CSINodeNamespace:        s.config.CSINodeNamespace,
		CSINodePort:             s.config.CSINodePort,
		WakeupRequestTTL:        s.config.WakeupRequestTTL,
		MaxConcurrentReconciles: s.config.WakeupMaxConcurrent,
	}
	if err := wakeupReconciler.SetupWithManager(ctx, mgr); err != nil {
		return err
	}

	s.manager = mgr
	return nil
}

// Setup implements service.Service.
func (s *BranchOperatorService) Setup(ctx context.Context) error {
	// This is a stateless service, nothing to setup
	return nil
}

// Close implements service.Service.
func (s *BranchOperatorService) Close(ctx context.Context) error {
	return nil
}

// Run implements service.RunnerService.
func (s *BranchOperatorService) Run(ctx context.Context, o *o11y.O) error {
	logger := o.Logger()

	// Set up controller-runtime logger
	ctrlLogger := logger.With().Str("module", "controller-runtime").Logger()
	ctrl.SetLogger(zerologr.New(&ctrlLogger))

	logger.Info().Msg("branch-operator starting")

	// Start the controller manager
	if err := s.manager.Start(ctx); err != nil {
		logger.Error().Err(err).Msg("branch-operator failed")
		return err
	}

	logger.Info().Msg("branch-operator stopping")
	return nil
}
