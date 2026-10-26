package restlua_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	restlua "github.com/taybart/rest/lua"
	lua "github.com/yuin/gopher-lua"
)

func newLuaState(t *testing.T) *lua.LState {
	t.Helper()
	l := lua.NewState()
	t.Cleanup(l.Close)
	if err := restlua.RegisterModules(l); err != nil {
		t.Fatalf("RegisterModules: %v", err)
	}
	return l
}

func TestLuaJWTHMAC(t *testing.T) {
	l := newLuaState(t)

	code := `
		local token = jwt.sign({ sub = "123", role = "admin" }, "super-secret", { expires_in = 60, header = { kid = "k1" } })
		assert(jwt.is_valid(token), "expected valid jwt structure")
		assert(jwt.is_valid(token, "super-secret"), "expected valid jwt signature")
		assert(not jwt.is_valid(token, "wrong-secret"), "expected invalid with wrong secret")

		local claims, header = jwt.decode(token)
		assert(claims.sub == "123", "unexpected sub")
		assert(claims.role == "admin", "unexpected role")
		assert(header.alg == "HS256", "unexpected alg")
		assert(header.kid == "k1", "unexpected kid")
		assert(type(claims.exp) == "number", "expected numeric exp")

		local vclaims, vheader = jwt.verify(token, "super-secret")
		assert(vclaims ~= nil, "expected verify to succeed")
		assert(vclaims.sub == "123", "unexpected verified sub")
		assert(vheader.alg == "HS256", "unexpected verified alg")

		local bad, err = jwt.verify(token, "wrong-secret")
		assert(bad == nil and err ~= nil, "expected verify with wrong secret to fail")
	`
	if err := l.DoString(code); err != nil {
		t.Fatalf("lua execution failed: %v", err)
	}
}

func TestLuaJWTExpiration(t *testing.T) {
	l := newLuaState(t)

	past := float64(time.Now().Unix() - 10)
	token, err := restlua.SignJWT(map[string]any{"sub": "u1", "exp": past}, "sec", restlua.JWTOptions{})
	if err != nil {
		t.Fatal(err)
	}
	l.SetGlobal("expired_token", lua.LString(token))

	code := `
		local claims, err = jwt.verify(expired_token, "sec")
		assert(claims == nil, "expected expired token to fail verification")
		assert(err:find("expired") ~= nil, "expected expired error message, got: " .. tostring(err))

		local ok_claims, ok_header = jwt.verify(expired_token, "sec", { ignore_exp = true })
		assert(ok_claims ~= nil and ok_claims.sub == "u1", "expected ignore_exp to allow expired token")
	`
	if err := l.DoString(code); err != nil {
		t.Fatalf("lua execution failed: %v", err)
	}
}

func TestJWTAsymmetricAlgorithms(t *testing.T) {
	// 1. RS256
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPrivPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaPriv),
	})
	rsaPubBytes, err := x509.MarshalPKIXPublicKey(&rsaPriv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	rsaPubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: rsaPubBytes,
	})

	tokRS, err := restlua.SignJWT(map[string]any{"sub": "rsa-user"}, string(rsaPrivPEM), restlua.JWTOptions{Algorithm: "RS256"})
	if err != nil {
		t.Fatalf("RS256 sign failed: %v", err)
	}
	claims, _, err := restlua.VerifyJWT(tokRS, string(rsaPubPEM), restlua.JWTOptions{Algorithm: "RS256"})
	if err != nil || claims["sub"] != "rsa-user" {
		t.Fatalf("RS256 verify failed: %v (%v)", err, claims)
	}

	// 2. ES256
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecPrivBytes, err := x509.MarshalECPrivateKey(ecPriv)
	if err != nil {
		t.Fatal(err)
	}
	ecPrivPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: ecPrivBytes,
	})
	ecPubBytes, err := x509.MarshalPKIXPublicKey(&ecPriv.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	ecPubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: ecPubBytes,
	})

	tokES, err := restlua.SignJWT(map[string]any{"sub": "ec-user"}, string(ecPrivPEM), restlua.JWTOptions{Algorithm: "ES256"})
	if err != nil {
		t.Fatalf("ES256 sign failed: %v", err)
	}
	claims, _, err = restlua.VerifyJWT(tokES, string(ecPubPEM), restlua.JWTOptions{Algorithm: "ES256"})
	if err != nil || claims["sub"] != "ec-user" {
		t.Fatalf("ES256 verify failed: %v (%v)", err, claims)
	}

	// 3. EdDSA
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edPrivBytes, err := x509.MarshalPKCS8PrivateKey(edPriv)
	if err != nil {
		t.Fatal(err)
	}
	edPrivPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: edPrivBytes,
	})
	edPubBytes, err := x509.MarshalPKIXPublicKey(edPub)
	if err != nil {
		t.Fatal(err)
	}
	edPubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: edPubBytes,
	})

	tokEd, err := restlua.SignJWT(map[string]any{"sub": "ed-user"}, string(edPrivPEM), restlua.JWTOptions{Algorithm: "EdDSA"})
	if err != nil {
		t.Fatalf("EdDSA sign failed: %v", err)
	}
	claims, _, err = restlua.VerifyJWT(tokEd, string(edPubPEM), restlua.JWTOptions{Algorithm: "EdDSA"})
	if err != nil || claims["sub"] != "ed-user" {
		t.Fatalf("EdDSA verify failed: %v (%v)", err, claims)
	}
}
