package gateway

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"

	"xata/internal/envcfg"
	"xata/internal/o11y"
	"xata/internal/service"
	"xata/services/gateway/initiator"
	"xata/services/gateway/ipfiltering"
	"xata/services/gateway/metrics"
	"xata/services/gateway/serverless"
	"xata/services/gateway/session"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Ensure GatewayService implements Service and RunnerService interfaces.
var (
	_ service.Service       = (*GatewayService)(nil)
	_ service.RunnerService = (*GatewayService)(nil)
)

type GatewayService struct {
	config      Config
	cliConfig   *CLIConfig
	certificate *tls.Certificate
	resolver    session.BranchResolver
	ipFilter    *ipfiltering.Filter
}

// NewGatewayService creates a new instance of the service.
func NewGatewayService(cliConfig *CLIConfig) *GatewayService {
	return &GatewayService{
		cliConfig: cliConfig,
	}
}

func (g *GatewayService) Name() string {
	return "gateway"
}

func (g *GatewayService) ReadConfig(ctx context.Context) error {
	return envcfg.Read(&g.config)
}

func (g *GatewayService) Init(ctx context.Context) error {
	if err := g.config.Validate(); err != nil {
		return err
	}

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("get in-cluster config: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("create kubernetes client: %w", err)
	}

	ipFilter, err := ipfiltering.NewFilter(ctx, kubeClient, g.config.XataNamespace, g.config.BranchesConfigMapName)
	if err != nil {
		return fmt.Errorf("init IP filter: %w", err)
	}
	g.ipFilter = ipFilter

	cert, err := loadCertificate(g.config.SSLCertPath, g.config.SSLKeyPath)
	if err != nil {
		return err
	}
	g.certificate = cert

	if g.cliConfig.DevPostgresURL != "" {
		g.resolver = session.ResolverFunc(func(ctx context.Context, serverName, fallbackEndpoint string) (*session.Branch, error) {
			return &session.Branch{
				ID:      "dev",
				Address: g.cliConfig.DevPostgresURL,
			}, nil
		})
	} else {
		g.resolver = session.NewCNPGBranchResolver(g.config.CNPGNamespace, g.config.TargetPort, g.config.EnablePooler)
	}

	return nil
}

func (g *GatewayService) Setup(ctx context.Context) error {
	return nil
}

func (g *GatewayService) Close(ctx context.Context) error {
	return nil
}

func (g *GatewayService) Run(ctx context.Context, o *o11y.O) error {
	const instrumentationName = "gateway"
	tracer := o.Tracer(instrumentationName)

	gwMetrics, err := metrics.New(o.Meter(instrumentationName))
	if err != nil {
		return fmt.Errorf("create gateway metrics: %w", err)
	}

	dialer := session.NewClusterDialer(
		session.ClusterDialerConfiguration{
			ReactivateTimeout:   g.config.ClusterReactivateTimeout,
			StatusCheckInterval: g.config.ClusterStatusCheckInterval,
		},
		session.WithInstrumentation(gwMetrics),
	)

	gwServer, err := g.newServer(gwMetrics, tracer, dialer)
	if err != nil {
		return err
	}

	if !g.config.HTTPEnabled {
		return gwServer.Run(ctx)
	}

	httpServer, err := serverless.NewServer(o, g.resolver, dialer, gwMetrics, g.ipFilter, serverless.Config{
		ListenAddress: g.config.HTTPListenAddress,
		TLSCert:       g.certificate,
	})
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error { return gwServer.Run(ctx) })
	eg.Go(func() error { return httpServer.Run(ctx) })

	return eg.Wait()
}

func (g *GatewayService) newServer(gwMetrics *metrics.GatewayMetrics, tracer trace.Tracer, dialer *session.ClusterDialer) (Server, error) {
	proxy := session.NewProxy(tracer, g.resolver, dialer.Dial, g.ipFilter)
	sessionInitiator, err := initiator.New(tracer, proxy, g.certificate)
	if err != nil {
		return nil, err
	}

	return NewServer(sessionInitiator, ServerConfig{
		Listen:       g.config.ListenAddress,
		DrainingTime: g.config.DrainingTime,
	}, gwMetrics), nil
}

func loadCertificate(certPath, keyPath string) (*tls.Certificate, error) {
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("TLS certificate and key paths are required")
	}

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read TLS certificate: %w", err)
	}

	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read TLS private key: %w", err)
	}

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load TLS certificate: %w", err)
	}
	return &cert, nil
}
