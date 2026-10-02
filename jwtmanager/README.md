# jwtmanager

Ed25519-signed access/refresh JWTs via `golang-jwt/jwt/v5`.

```go
priv, err := jwtmanager.LoadPrivateKey("keys/ed25519.priv.pem")
pub, err := jwtmanager.LoadPublicKey("keys/ed25519.pub.pem")

svc, err := jwtmanager.New(priv, pub, jwtmanager.Config{
    AccessTokenDuration:  15 * time.Minute,
    RefreshTokenDuration: 7 * 24 * time.Hour,
    Issuer:               "gorest",
})

access, err := svc.GenerateAccessToken(userID, []string{"admin"})
refresh, err := svc.GenerateRefreshToken(userID)

claims, err := svc.Verify(access, jwtmanager.TokenTypeAccess)
// claims.Subject, claims.Roles, claims.ExpiresAt, claims.TokenType
```

Refresh tokens carry only subject + type (least-privilege). `Verify` rejects
wrong token type, wrong signature, unexpected alg, and expired tokens.

Generate a keypair:

```sh
openssl genpkey -algorithm ed25519 -out ed25519.priv.pem
openssl pkey -in ed25519.priv.pem -pubout -out ed25519.pub.pem
```
