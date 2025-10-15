package main

import (
	"fmt"
	"strings"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/service/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/metal-stack/metal-lib/pkg/pointer"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	tokenSubjectFlag = &cli.StringFlag{
		Name:  "subject",
		Value: "metal-stack",
		Usage: "requested subject for the token (should be present in the database)",
	}
	tokenDescriptionFlag = &cli.StringFlag{
		Name:  "description",
		Value: "",
		Usage: "the description for what this token is going to be used",
	}
	tokenPermissionsFlag = &cli.StringSliceFlag{
		Name:  "permissions",
		Value: &cli.StringSlice{},
		Usage: "requested permissions for the token",
	}
	tokenProjectRolesFlag = &cli.StringSliceFlag{
		Name:  "project-roles",
		Value: &cli.StringSlice{},
		Usage: "requested project roles for the token",
	}
	tokenTenantRolesFlag = &cli.StringSliceFlag{
		Name:  "tenant-roles",
		Value: &cli.StringSlice{},
		Usage: "requested tenant roles for the token",
	}
	tokenAdminRoleFlag = &cli.StringFlag{
		Name:  "admin-role",
		Value: "",
		Usage: "requested admin role for the token",
	}
	tokenExpirationFlag = &cli.DurationFlag{
		Name:  "expiration",
		Value: 6 * 30 * 24 * time.Hour,
		Usage: "requested expiration for the token",
	}
)

func newTokenCmd() *cli.Command {
	return &cli.Command{
		Name:  "token",
		Usage: "create api tokens for cloud infrastructure services that depend on the api-server like accounting, status dashboard, ...",
		Flags: []cli.Flag{
			logLevelFlag,
			redisAddrFlag,
			redisPasswordFlag,
			tokenSubjectFlag,
			tokenDescriptionFlag,
			tokenPermissionsFlag,
			tokenProjectRolesFlag,
			tokenTenantRolesFlag,
			tokenAdminRoleFlag,
			tokenExpirationFlag,
			serverHttpUrlFlag,
		},
		Action: func(ctx *cli.Context) error {
			log, err := createLogger(ctx)
			if err != nil {
				return fmt.Errorf("unable to create logger %w", err)
			}

			tokenRedisClient, err := createRedisClient(ctx, log, redisDatabaseTokens)
			if err != nil {
				return err
			}

			tokenStore := tokencommon.NewRedisStore(tokenRedisClient)
			certStore := certs.NewRedisStore(&certs.Config{
				RedisClient: tokenRedisClient,
			})

			tokenService := token.New(token.Config{
				Log:        log,
				TokenStore: tokenStore,
				CertStore:  certStore,
				Issuer:     ctx.String(serverHttpUrlFlag.Name),
			})

			var permissions []*apiv2.MethodPermission
			for _, m := range ctx.StringSlice(tokenPermissionsFlag.Name) {
				project, semicolonSeparatedMethods, ok := strings.Cut(m, "=")
				if !ok {
					return fmt.Errorf("permissions must be provided in the form <project>=<methods-colon-separated>")
				}

				permissions = append(permissions, &apiv2.MethodPermission{
					Subject: project,
					Methods: strings.Split(semicolonSeparatedMethods, ":"),
				})
			}

			projectRoles := map[string]apiv2.ProjectRole{}
			for _, r := range ctx.StringSlice(tokenProjectRolesFlag.Name) {
				projectID, roleString, ok := strings.Cut(r, "=")
				if !ok {
					return fmt.Errorf("project roles must be provided in the form <project-id>=<role>")
				}

				role, ok := apiv2.ProjectRole_value[roleString]
				if !ok {
					return fmt.Errorf("unknown role: %s", roleString)
				}

				projectRoles[projectID] = apiv2.ProjectRole(role)
			}

			tenantRoles := map[string]apiv2.TenantRole{}
			for _, r := range ctx.StringSlice(tokenTenantRolesFlag.Name) {
				tenantID, roleString, ok := strings.Cut(r, "=")
				if !ok {
					return fmt.Errorf("tenant roles must be provided in the form <tenant-id>=<role>")
				}

				role, ok := apiv2.TenantRole_value[roleString]
				if !ok {
					return fmt.Errorf("unknown role: %s", roleString)
				}

				tenantRoles[tenantID] = apiv2.TenantRole(role)
			}

			var adminRole *apiv2.AdminRole
			if roleString := ctx.String(tokenAdminRoleFlag.Name); roleString != "" {
				role, ok := apiv2.AdminRole_value[roleString]
				if !ok {
					return fmt.Errorf("unknown role: %s", roleString)
				}

				adminRole = pointer.Pointer(apiv2.AdminRole(role))
			}
			subject := ctx.String(tokenSubjectFlag.Name)
			if subject == "" {
				return fmt.Errorf("token subject cannot be empty")
			}

			resp, err := tokenService.CreateApiTokenWithoutPermissionCheck(ctx.Context, subject, &apiv2.TokenServiceCreateRequest{
				Description:  ctx.String(tokenDescriptionFlag.Name),
				Expires:      durationpb.New(ctx.Duration(tokenExpirationFlag.Name)),
				ProjectRoles: projectRoles,
				TenantRoles:  tenantRoles,
				AdminRole:    adminRole,
				Permissions:  permissions,
			})
			if err != nil {
				return err
			}

			fmt.Println(resp.Secret)

			return nil
		},
	}
}
