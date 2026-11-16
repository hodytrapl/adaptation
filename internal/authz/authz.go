package authz

import (
	"context"
	"errors"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalidToken = errors.New("authz: invalid token")

type Claims struct {
	jwt.RegisteredClaims
	UserID int64    `json:"uid"`
	Role   string   `json:"role"`
	Scopes []string `json:"scopes"`
	Type   string   `json:"typ"`
}

func (c *Claims) HasScope(want string) bool {
	for _, s := range c.Scopes {
		if s == want || s == "*:*" {
			return true
		}
		if strings.HasSuffix(want, ":read") || strings.HasSuffix(want, ":write") {
			module := want[:strings.LastIndex(want, ":")]
			if s == module+":admin" {
				return true
			}
		}
	}
	return false
}

type Verifier interface {
	VerifyAccess(token string) (*Claims, error)
}

type ctxKey string

const claimsKey ctxKey = "auth_claims"

func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func ClaimsFrom(ctx context.Context) (*Claims, bool) {
	c, ok := ctx.Value(claimsKey).(*Claims)
	return c, ok
}