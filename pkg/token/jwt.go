package token

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"slices"
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
			ID: uuid.New().String(),

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
	_, _, err := new(jwt.Parser).ParseUnverified(string(token), claims)

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

func Validate(ctx context.Context, tokenString string, set jwk.Set, allowedIssuers []string) (*Claims, error) {
	claims := &Claims{}

	var parseErrors []error

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		// Try each key in the set
		for i := 0; i < set.Len(); i++ {
			key, ok := set.Key(i)
			if !ok {
				parseErrors = append(parseErrors, fmt.Errorf("failed to get key at index %d", i))
				continue
			}

			if !isKeyValid(key) {
				continue
			}

			var rawKey any
			if err := jwk.Export(key, &rawKey); err != nil {
				parseErrors = append(parseErrors, err)
				continue
			}

			return rawKey, nil
		}
		return nil, fmt.Errorf("no suitable key found: %v", errors.Join(parseErrors...))
	}, jwt.WithValidMethods([]string{jwt.SigningMethodES512.Alg()})) // TODO parse alg from token
	if err != nil {
		return nil, err
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

func isKeyValid(key jwk.Key) bool {
	// Check for expiration (custom claim, not standard)
	var exp any
	if err := key.Get("exp", &exp); err != nil {
		if expTime, ok := exp.(float64); ok {
			return time.Now().Unix() < int64(expTime)
		}
	}
	return false // No expiration set, assume invalid
}
