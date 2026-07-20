package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"buf.build/go/protoyaml"
	adminv2 "github.com/metal-stack/api/go/metalstack/admin/v2"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/api/go/permissions"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/k8s"
	"github.com/metal-stack/metal-apiserver/pkg/repository"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/urfave/cli/v2"
	"go.yaml.in/yaml/v3"
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
	tokenInfraRoleFlag = &cli.StringFlag{
		Name:  "infra-role",
		Value: "",
		Usage: "requested infra role for the token",
	}
	tokenMachineRolesFlag = &cli.StringSliceFlag{
		Name:  "machine-roles",
		Value: &cli.StringSlice{},
		Usage: "requested machine roles for the token",
	}
	tokenExpirationFlag = &cli.DurationFlag{
		Name:  "expiration",
		Value: 6 * 30 * 24 * time.Hour,
		Usage: "requested expiration for the token",
	}
	namespaceFlag = &cli.StringFlag{
		Name:    "namespace",
		Value:   "metal-control-plane",
		Usage:   "namespace of the secret",
		EnvVars: []string{"NAMESPACE"},
	}
	secretNameFlag = &cli.StringFlag{
		Name:    "secret-name",
		Value:   "metal-apiserver-admin-token",
		Usage:   "name of the secret",
		EnvVars: []string{"SECRET_NAME"},
	}
	tokensCreateConfigFileFlag = &cli.StringFlag{
		Name:    "tokens-create-config-file",
		Value:   "",
		Usage:   "path to a yaml file which contains the serialized map from token-name to TokenServiceCreateRequest. If provided the generated tokens will not be printed to stdout but instead written back into a kubernetes secret resource as specified by the secret-name and namespace flags.",
		EnvVars: []string{"TOKENS_CREATE_CONFIG_FILE_PATH"},
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
			providerTenantFlag,
			tokenSubjectFlag,
			tokenDescriptionFlag,
			tokenPermissionsFlag,
			tokenProjectRolesFlag,
			tokenTenantRolesFlag,
			tokenAdminRoleFlag,
			tokenInfraRoleFlag,
			tokenMachineRolesFlag,
			tokenExpirationFlag,
			serverHttpUrlFlag,
			namespaceFlag,
			secretNameFlag,
			tokensCreateConfigFileFlag,
		},
		Action: func(ctx *cli.Context) error {
			log, err := createLogger(ctx)
			if err != nil {
				return fmt.Errorf("unable to create logger %w", err)
			}

			tokenRedisClient, _, err := createRedisClient(ctx, log, redisDatabaseTokens)
			if err != nil {
				return err
			}

			repo := repository.New(repository.Config{
				Log: log,
				TokenConfig: repository.TokenConfig{
					TokenStore: tokencommon.NewRedisStore(tokenRedisClient),
					CertStore: certs.NewRedisStore(&certs.Config{
						RedisClient: tokenRedisClient,
					}),
					ProviderTenant: ctx.String(providerTenantFlag.Name),
					Issuer:         ctx.String(serverHttpUrlFlag.Name),
				},
			})

			var typedPermissions []*apiv2.PermissionsByVisibility
			for _, m := range ctx.StringSlice(tokenPermissionsFlag.Name) {
				subject, colonSeparatedMethods, ok := strings.Cut(m, "=")
				if !ok {
					return fmt.Errorf("permissions must be provided in the form [<subject>=<methods-colon-separated>")
				}

				for _, method := range strings.Split(colonSeparatedMethods, ":") {
					if _, ok := permissions.GetServicePermissions().Visibility.Admin[method]; ok {

						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Admin{
								Admin: &apiv2.AdminPermissions{
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Infra[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Infra{
								Infra: &apiv2.InfraPermissions{
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Machine[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Machine{
								Machine: &apiv2.MachinePermissions{
									Uuid:    subject,
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Project[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Project{
								Project: &apiv2.ProjectPermissions{
									Project: subject,
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Public[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Public{
								Public: &apiv2.PublicPermissions{
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Self[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Self{
								Self: &apiv2.SelfPermissions{
									Methods: []string{method},
								},
							},
						})

						continue
					}

					if _, ok := permissions.GetServicePermissions().Visibility.Tenant[method]; ok {
						typedPermissions = append(typedPermissions, &apiv2.PermissionsByVisibility{
							Visibility: &apiv2.PermissionsByVisibility_Tenant{
								Tenant: &apiv2.TenantPermissions{
									Login:   subject,
									Methods: []string{method},
								},
							},
						})

						continue
					}

					return fmt.Errorf("your requested method is not part of the api: %s", method)
				}
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

				adminRole = new(apiv2.AdminRole(role))
			}
			var infraRole *apiv2.InfraRole
			if roleString := ctx.String(tokenInfraRoleFlag.Name); roleString != "" {
				role, ok := apiv2.InfraRole_value[roleString]
				if !ok {
					return fmt.Errorf("unknown role: %s", roleString)
				}

				infraRole = new(apiv2.InfraRole(role))
			}
			machineRoles := map[string]apiv2.MachineRole{}
			for _, r := range ctx.StringSlice(tokenMachineRolesFlag.Name) {
				machineID, roleString, ok := strings.Cut(r, "=")
				if !ok {
					return fmt.Errorf("machine roles must be provided in the form <machine-uuid>=<role>")
				}

				role, ok := apiv2.MachineRole_value[roleString]
				if !ok {
					return fmt.Errorf("unknown role: %s", roleString)
				}

				machineRoles[machineID] = apiv2.MachineRole(role)
			}
			subject := ctx.String(tokenSubjectFlag.Name)
			if subject == "" {
				return fmt.Errorf("token subject cannot be empty")
			}

			if configFile := ctx.String(tokensCreateConfigFileFlag.Name); configFile != "" {
				namespace := ctx.String(namespaceFlag.Name)
				if namespace == "" {
					return fmt.Errorf("namespace cannot be empty")
				}

				secretName := ctx.String(secretNameFlag.Name)
				if secretName == "" {
					return fmt.Errorf("secretName cannot be empty")
				}

				providerTenant := ctx.String(providerTenantFlag.Name)
				if providerTenant == "" {
					return fmt.Errorf("providerTenant cannot be empty")
				}

				return storeTokensFromConfigFile(ctx.Context, log, repo, configFile, providerTenant, namespace, secretName)
			}

			resp, err := repo.UnscopedToken().AdditionalMethods().CreateApiTokenWithoutPermissionCheck(ctx.Context, subject, &apiv2.TokenServiceCreateRequest{
				Description:  ctx.String(tokenDescriptionFlag.Name),
				Expires:      durationpb.New(ctx.Duration(tokenExpirationFlag.Name)),
				ProjectRoles: projectRoles,
				TenantRoles:  tenantRoles,
				AdminRole:    adminRole,
				Permissions:  typedPermissions,
				InfraRole:    infraRole,
				MachineRoles: machineRoles,
			})
			if err != nil {
				return err
			}

			fmt.Println(resp.Secret)

			return nil
		},
	}
}

func storeTokensFromConfigFile(ctx context.Context, log *slog.Logger, repo *repository.Store, configFile, providerTenant, namespace, secretName string) error {
	yamlBytes, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	var config map[string]any
	err = yaml.Unmarshal(yamlBytes, &config)
	if err != nil {
		return err
	}

	for target, v := range config {
		unmodifiedProtoBytes, err := yaml.Marshal(v)
		if err != nil {
			return fmt.Errorf("unable to marshal yaml content from target:%s %w", target, err)
		}

		var tokenCreateRequest adminv2.TokenServiceCreateRequest
		err = protoyaml.Unmarshal(unmodifiedProtoBytes, &tokenCreateRequest)
		if err != nil {
			return fmt.Errorf("unable to unmarshal protoyaml from target:%s %w", target, err)
		}

		subject := providerTenant
		if tokenCreateRequest.User != nil {
			subject = *tokenCreateRequest.User
		}

		resp, err := repo.UnscopedToken().AdditionalMethods().CreateApiTokenWithoutPermissionCheck(ctx, subject, tokenCreateRequest.TokenCreateRequest)
		if err != nil {
			return err
		}

		log.Info("store token in secret", "namespace", namespace, "secret-name", secretName)
		err = k8s.CreateOrUpdateSecret(ctx, log, namespace, applicationName, secretName, target, resp.Secret)
		if err != nil {
			return err
		}
	}

	return nil
}
