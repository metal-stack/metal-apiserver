package admin

import (
	"log/slog"

	"connectrpc.com/connect/v2"
	"github.com/metal-stack/api/go/metalstack/admin/v2/adminv2connect"
	"github.com/metal-stack/metal-apiserver/pkg/invite"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	"github.com/metal-stack/metal-lib/auditing"

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
	Log                *slog.Logger
	Repository         *repository.Store
	Server             *connect.Server
	InviteStore        invite.TenantInviteStore
	AuditSearchBackend auditing.Auditing
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
		})
		adminTokenService = tokenadmin.New(tokenadmin.Config{
			Log:  cfg.Log,
			Repo: cfg.Repository,
		})
		adminVPNService = vpnadmin.New(vpnadmin.Config{
			Log:  cfg.Log,
			Repo: cfg.Repository,
		})
	)

	adminv2connect.RegisterAuditServiceHandler(cfg.Server, adminAuditService)
	adminv2connect.RegisterComponentServiceHandler(cfg.Server, adminComponentService)
	adminv2connect.RegisterFilesystemServiceHandler(cfg.Server, adminFilesystemService)
	adminv2connect.RegisterImageServiceHandler(cfg.Server, adminImageService)
	adminv2connect.RegisterIPServiceHandler(cfg.Server, adminIpService)
	adminv2connect.RegisterMachineServiceHandler(cfg.Server, adminMachineService)
	adminv2connect.RegisterNetworkServiceHandler(cfg.Server, adminNetworkService)
	adminv2connect.RegisterPartitionServiceHandler(cfg.Server, adminPartitionService)
	adminv2connect.RegisterProjectServiceHandler(cfg.Server, adminProjectService)
	adminv2connect.RegisterSizeImageConstraintServiceHandler(cfg.Server, adminSizeImageConstraintService)
	adminv2connect.RegisterSizeReservationServiceHandler(cfg.Server, adminSizeReservationService)
	adminv2connect.RegisterSizeServiceHandler(cfg.Server, adminSizeService)
	adminv2connect.RegisterSwitchServiceHandler(cfg.Server, adminSwitchService)
	adminv2connect.RegisterTaskServiceHandler(cfg.Server, adminTaskService)
	adminv2connect.RegisterTenantServiceHandler(cfg.Server, adminTenantService)
	adminv2connect.RegisterTokenServiceHandler(cfg.Server, adminTokenService)
	adminv2connect.RegisterVPNServiceHandler(cfg.Server, adminVPNService)
}
