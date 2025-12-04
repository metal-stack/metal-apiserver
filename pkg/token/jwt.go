package token

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwk"
	apiv2 "github.com/metal-stack/api/go/metalstack/api/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	DefaultExpiration = time.Hour * 8
	MaxExpiration     = 365 * 24 * time.Hour
)

type (
	Claims struct {
		jwt.RegisteredClaims

		Type string `json:"type"`
	}

	tokenContextKey struct{}
)

func NewJWT(tokenType apiv2.TokenType, subject, issuer string, expires time.Duration, secret crypto.PrivateKey) (string, *apiv2.Token, error) {
	if expires == 0 {
		expires = DefaultExpiration
	}
	if expires > MaxExpiration {
		return "", nil, fmt.Errorf("expires: %q exceeds maximum: %q", expires, MaxExpiration)
	}

	id, err := uuid.NewV7()
	if err != nil {
		return "", nil, err
	}

	issuedAt := time.Now().UTC()
	expiresAt := issuedAt.Add(expires)
	claims := &Claims{
		// see overview of "registered" JWT claims as used by jwt-go here:
		//   https://pkg.go.dev/github.com/golang-jwt/jwt/v5?utm_source=godoc#RegisteredClaims
		// see the semantics of the registered claims here:
		//   https://en.wikipedia.org/wiki/JSON_Web_Token#Standard_fields
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(issuedAt),
			NotBefore: jwt.NewNumericDate(issuedAt),

			// ID is for your traceability, doesn't have to be UUID:
			ID: id.String(),

			// put name/title/ID of whoever will be using this JWT here:
			Subject: subject,
			Issuer:  issuer,
		},
		Type: tokenType.String(),
	}

	jwtWithClaims := jwt.NewWithClaims(jwt.SigningMethodES512, claims)
	res, err := jwtWithClaims.SignedString(secret)
	if err != nil {
		return "", nil, fmt.Errorf("unable to sign ES512 JWT: %w", err)
	}

	token := &apiv2.Token{
		Uuid:      claims.ID,
		User:      subject,
		Expires:   timestamppb.New(expiresAt),
		IssuedAt:  timestamppb.New(issuedAt),
		TokenType: tokenType,
	}

	return res, token, nil
}

// ParseJWTToken unverified to Claims to get Issuer,Subject, Roles and Permissions
func ParseJWTToken(token string) (*Claims, error) {
	if token == "" {
		return nil, nil
	}

	claims := &Claims{}
	parser := jwt.NewParser()
	_, _, err := parser.ParseUnverified(string(token), claims)

	if err != nil {
		return nil, err
	}

	return claims, nil
}

// ContextWithToken stores the token in the Context
// Can later retrieved with TokenFromContext
func ContextWithToken(ctx context.Context, token *apiv2.Token) context.Context {
	return context.WithValue(ctx, tokenContextKey{}, token)
}

// TokenFromContext retrieves the token and ok from the context
// if previously stored by calling ContextWithToken.
func TokenFromContext(ctx context.Context) (*apiv2.Token, bool) {
	value := ctx.Value(tokenContextKey{})

	token, ok := value.(*apiv2.Token)

	return token, ok
}

func Validate(ctx context.Context, log *slog.Logger, tokenString string, set jwk.Set, allowedIssuers []string) (*Claims, error) {
	var (
		claims      = &Claims{}
		publicKey   crypto.PublicKey
		parseErrors []error
		token       *jwt.Token
		keyFunc     = func(t *jwt.Token) (any, error) { return publicKey, nil }
	)

	parser := jwt.NewParser()
	t, _, err := parser.ParseUnverified(tokenString, claims)
	if err != nil {
		return nil, err
	}
	t.Method.Alg()

	log.Debug("validate", "tokenstring", tokenString)
	for i := range set.Len() {
		key, ok := set.Key(i)
		if !ok {
			continue
		}

		err := jwk.Export(key, &publicKey)
		if err != nil {
			log.Error("unable to export publickey", "error", err)
			continue
		}

		unvalidatedTok, err := jwt.ParseWithClaims(tokenString, claims, keyFunc, jwt.WithValidMethods([]string{t.Method.Alg()}))
		if err != nil {
			if errors.Is(err, jwt.ErrTokenSignatureInvalid) {
				// If many public keys are present to verify jwt token signatures, many of them might be invalid.
				// skip those which throw a token signature invalid error to be able to try the next one.
				if strings.HasSuffix(tokenString, "foo") {
					log.Debug("validate", "error", err)
				}
				continue
			}
			parseErrors = append(parseErrors, err)
		}
		if err == nil {
			token = unvalidatedTok
		}
	}

	if strings.HasSuffix(tokenString, "foo") {
		log.Debug("validate", "token", token)
	}

	if len(parseErrors) > 0 {
		return nil, errors.Join(parseErrors...)
	}

	if token == nil {
		return nil, fmt.Errorf("token is not valid, no suitable publickey to validate signature found")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("unknown claims type %T, cannot proceed", token.Claims)
	}

	if !slices.Contains(allowedIssuers, claims.Issuer) {
		return nil, fmt.Errorf("invalid token issuer: %s", claims.Issuer)
	}

	return claims, nil
}
