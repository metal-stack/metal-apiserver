package main

import (
	"fmt"
	"time"

	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"github.com/metal-stack/metal-apiserver/pkg/certs"
	"github.com/metal-stack/metal-apiserver/pkg/k8s"
	"github.com/metal-stack/metal-apiserver/pkg/service/api/token"
	tokencommon "github.com/metal-stack/metal-apiserver/pkg/token"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	namespaceFlag = &cli.StringFlag{
		Name:  "namespace",
		Value: "metal-control-plane",
		Usage: "namespace of the secret",
	}
	secretNameFlag = &cli.StringFlag{
		Name:  "secret-name",
		Value: "metal-apiserver-admin-token",
		Usage: "name of the secret",
	}
	adminTokenExpirationFlag = &cli.DurationFlag{
		Name:  "expiration",
		Value: 6 * 30 * 24 * time.Hour,
		Usage: "requested expiration for the token",
	}
)

func newAdminTokenCmd() *cli.Command {
	return &cli.Command{
		Name:  "admin-token",
		Usage: "create a admin token command and store it in a secret",
		Flags: []cli.Flag{
			logLevelFlag,
			redisAddrFlag,
			redisPasswordFlag,
			serverHttpUrlFlag,
			tokenSubjectFlag,
			tokenDescriptionFlag,
			adminTokenExpirationFlag,
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

			subject := ctx.String(tokenSubjectFlag.Name)
			if subject == "" {
				return fmt.Errorf("token subject cannot be empty")
			}

			namespace := ctx.String(namespaceFlag.Name)
			if namespace == "" {
				return fmt.Errorf("namespace cannot be empty")
			}

			secretName := ctx.String(secretNameFlag.Name)
			if secretName == "" {
				return fmt.Errorf("secretName cannot be empty")
			}

			resp, err := tokenService.CreateApiTokenWithoutPermissionCheck(ctx.Context, subject, &apiv2.TokenServiceCreateRequest{
				Description: ctx.String(tokenDescriptionFlag.Name),
				Expires:     durationpb.New(ctx.Duration(adminTokenExpirationFlag.Name)),
				AdminRole:   apiv2.AdminRole_ADMIN_ROLE_EDITOR.Enum(),
			})
			if err != nil {
				return err
			}
			return k8s.CreateOrUpdateSecret(ctx.Context, log, namespace, secretName, "admin-token", resp.Secret)
		},
	}
}
