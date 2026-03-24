package admin

import (
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/auditing"

	headscalev1 "github.com/juanfont/headscale/gen/go/headscale/v1"

	apitoken "github.com/metal-stack/metal-apiserver/pkg/service/api/token"

	auditadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/audit"
	componentadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/component"
	filesystemadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/filesystem"
	imageadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/image"
	ipadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/ip"
	machineadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/machine"
	networkadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/network"
	partitionadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/partition"
	projectadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/project"
	sizeadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size"
	sizeimageconstraintadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size-image-constraint"
	sizereservationadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/size-reservation"
	switchadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/switch"
	taskadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/task"
	tenantadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/tenant"
	tokenadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/token"
	vpnadmin "github.com/metal-stack/metal-apiserver/pkg/service/admin/vpn"
)

type Config struct {
	Log                          *slog.Logger
	Repository                   *repository.Store
	Mux                          *http.ServeMux
	Interceptors                 connect.Option
	InviteStore                  invite.TenantInviteStore
	TokenStore                   token.TokenStore
	TokenService                 apitoken.TokenService
	CertStore                    certs.CertStore
	HeadscaleClient              headscalev1.HeadscaleServiceClient
	HeadscaleControlplaneAddress string
	AuditSearchBackend           auditing.Auditing
}

func AdminServices(cfg Config) {

	var (
		adminAuditService               = auditadmin.New(auditadmin.Config{Log: cfg.Log, Repo: cfg.Repository, AuditClient: cfg.AuditSearchBackend})
		adminComponentService           = componentadmin.New(componentadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminFilesystemService          = filesystemadmin.New(filesystemadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminImageService               = imageadmin.New(imageadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminIpService                  = ipadmin.New(ipadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminMachineService             = machineadmin.New(machineadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminNetworkService             = networkadmin.New(networkadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminPartitionService           = partitionadmin.New(partitionadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminProjectService             = projectadmin.New(projectadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminSizeImageConstraintService = sizeimageconstraintadmin.New(sizeimageconstraintadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminSizeReservationService     = sizereservationadmin.New(sizereservationadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminSizeService                = sizeadmin.New(sizeadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminSwitchService              = switchadmin.New(switchadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminTaskService                = taskadmin.New(taskadmin.Config{Log: cfg.Log, Repo: cfg.Repository})
		adminTenantService              = tenantadmin.New(tenantadmin.Config{
			Log:         cfg.Log,
			Repo:        cfg.Repository,
			InviteStore: cfg.InviteStore,
			TokenStore:  cfg.TokenStore,
		})
		adminTokenService = tokenadmin.New(tokenadmin.Config{Log: cfg.Log, CertStore: cfg.CertStore, TokenStore: cfg.TokenStore, TokenService: cfg.TokenService})
	)

	cfg.Mux.Handle(adminv2connect.NewAuditServiceHandler(adminAuditService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewComponentServiceHandler(adminComponentService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewFilesystemServiceHandler(adminFilesystemService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewImageServiceHandler(adminImageService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewIPServiceHandler(adminIpService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewMachineServiceHandler(adminMachineService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewNetworkServiceHandler(adminNetworkService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewPartitionServiceHandler(adminPartitionService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewProjectServiceHandler(adminProjectService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewSizeImageConstraintServiceHandler(adminSizeImageConstraintService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewSizeReservationServiceHandler(adminSizeReservationService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewSizeServiceHandler(adminSizeService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewSwitchServiceHandler(adminSwitchService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewTaskServiceHandler(adminTaskService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewTenantServiceHandler(adminTenantService, cfg.Interceptors))
	cfg.Mux.Handle(adminv2connect.NewTokenServiceHandler(adminTokenService, cfg.Interceptors))

	if cfg.HeadscaleClient != nil {
		adminVPNService := vpnadmin.New(vpnadmin.Config{
			Log:                          cfg.Log,
			Repo:                         cfg.Repository,
			HeadscaleClient:              cfg.HeadscaleClient,
			HeadscaleControlplaneAddress: cfg.HeadscaleControlplaneAddress,
		})
		cfg.Mux.Handle(adminv2connect.NewVPNServiceHandler(adminVPNService))
	}
}
