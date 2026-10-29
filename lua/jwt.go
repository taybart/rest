package restlua

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"slices"
	"strings"
	"time"

	lua "github.com/taybart/go-erde"
)

// JWTOptions configures JWT signing and verification.
type JWTOptions struct {
	Algorithm  string
	Header     map[string]any
	ExpiresIn  float64 // relative seconds to set exp (and iat if not set)
	NoTimeValidation bool
}

func registerJWTModule(l *lua.LState) {
	mod := l.SetFuncs(l.NewTable(), map[string]lua.LGFunction{
		"sign":     luaJWTSign,
		"encode":   luaJWTSign,
		"decode":   luaJWTDecode,
		"verify":   luaJWTVerify,
		"is_valid": luaJWTIsValid,
	})
	l.SetGlobal("jwt", mod)
}

// IsJWT reports whether token has a valid 3-part JWT structure with decodable JSON header and payload.
func IsJWT(token string) bool {
	_, _, _, err := DecodeJWT(token)
	return err == nil
}

// DecodeJWT decodes a JWT without verifying the signature, returning (claims, header, signature, error).
func DecodeJWT(token string) (map[string]any, map[string]any, string, error) {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, "", errors.New("invalid jwt: expected 3 dot-separated parts")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid jwt header encoding: %w", err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, "", fmt.Errorf("invalid jwt header json: %w", err)
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid jwt claims encoding: %w", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, nil, "", fmt.Errorf("invalid jwt claims json: %w", err)
	}

	if _, err := base64.RawURLEncoding.DecodeString(parts[2]); err != nil {
		return nil, nil, "", fmt.Errorf("invalid jwt signature encoding: %w", err)
	}

	return claims, header, parts[2], nil
}

// SignJWT creates and signs a JWT string.
func SignJWT(claims map[string]any, secret string, opts JWTOptions) (string, error) {
	alg := strings.ToUpper(strings.TrimSpace(opts.Algorithm))
	if alg == "" {
		if hAlg, ok := opts.Header["alg"].(string); ok && hAlg != "" {
			alg = strings.ToUpper(strings.TrimSpace(hAlg))
		} else {
			alg = "HS256"
		}
	}

	header := map[string]any{
		"alg": alg,
		"typ": "JWT",
	}
	for k, v := range opts.Header {
		header[k] = v
	}
	header["alg"] = alg

	payload := make(map[string]any, len(claims)+2)
	for k, v := range claims {
		payload[k] = v
	}
	if opts.ExpiresIn > 0 {
		now := time.Now().Unix()
		if _, hasIAT := payload["iat"]; !hasIAT {
			payload["iat"] = float64(now)
		}
		if _, hasEXP := payload["exp"]; !hasEXP {
			payload["exp"] = float64(now) + opts.ExpiresIn
		}
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig, err := signBytes(alg, []byte(signingInput), secret)
	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyJWT verifies the signature and temporal claims (exp, nbf) of a JWT.
func VerifyJWT(token, secret string, opts JWTOptions) (map[string]any, map[string]any, error) {
	token = strings.TrimSpace(token)
	token = strings.TrimPrefix(token, "Bearer ")
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, errors.New("invalid jwt: expected 3 dot-separated parts")
	}

	claims, header, _, err := DecodeJWT(token)
	if err != nil {
		return nil, nil, err
	}

	alg, _ := header["alg"].(string)
	alg = strings.ToUpper(strings.TrimSpace(alg))
	if alg == "" || alg == "NONE" {
		return nil, nil, errors.New("invalid or unsigned jwt algorithm")
	}
	if opts.Algorithm != "" && !strings.EqualFold(opts.Algorithm, alg) {
		return nil, nil, fmt.Errorf("unexpected jwt algorithm %q (expected %q)", alg, opts.Algorithm)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid jwt signature encoding: %w", err)
	}

	signingInput := []byte(parts[0] + "." + parts[1])
	if err := verifyBytes(alg, signingInput, sigBytes, secret); err != nil {
		return nil, nil, err
	}

	if !opts.NoTimeValidation {
		now := float64(time.Now().Unix())
		if exp, ok := claimFloat(claims["exp"]); ok && now >= exp {
			return nil, nil, errors.New("jwt token is expired")
		}
		if nbf, ok := claimFloat(claims["nbf"]); ok && now < nbf {
			return nil, nil, errors.New("jwt token is not valid yet (nbf)")
		}
	}

	return claims, header, nil
}

func claimFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func hashForAlg(alg string) (crypto.Hash, func() hash.Hash, error) {
	switch alg {
	case "HS256", "RS256", "ES256":
		return crypto.SHA256, sha256.New, nil
	case "HS384", "RS384", "ES384":
		return crypto.SHA384, sha512.New384, nil
	case "HS512", "RS512", "ES512":
		return crypto.SHA512, sha512.New, nil
	default:
		return 0, nil, fmt.Errorf("unsupported jwt algorithm %q", alg)
	}
}

func signBytes(alg string, signingInput []byte, secret string) ([]byte, error) {
	switch alg {
	case "NONE":
		return []byte{}, nil
	case "HS256", "HS384", "HS512":
		_, newHash, err := hashForAlg(alg)
		if err != nil {
			return nil, err
		}
		mac := hmac.New(newHash, []byte(secret))
		mac.Write(signingInput)
		return mac.Sum(nil), nil
	case "RS256", "RS384", "RS512":
		cryptoHash, newHash, err := hashForAlg(alg)
		if err != nil {
			return nil, err
		}
		priv, err := parseRSAPrivateKey(secret)
		if err != nil {
			return nil, err
		}
		h := newHash()
		h.Write(signingInput)
		return rsa.SignPKCS1v15(rand.Reader, priv, cryptoHash, h.Sum(nil))
	case "ES256", "ES384", "ES512":
		_, newHash, err := hashForAlg(alg)
		if err != nil {
			return nil, err
		}
		priv, err := parseECDSAPrivateKey(secret)
		if err != nil {
			return nil, err
		}
		h := newHash()
		h.Write(signingInput)
		r, s, err := ecdsa.Sign(rand.Reader, priv, h.Sum(nil))
		if err != nil {
			return nil, err
		}
		keyBytes := (priv.Curve.Params().BitSize + 7) / 8
		out := make([]byte, 2*keyBytes)
		r.FillBytes(out[:keyBytes])
		s.FillBytes(out[keyBytes:])
		return out, nil
	case "EDDSA":
		priv, err := parseEd25519PrivateKey(secret)
		if err != nil {
			return nil, err
		}
		return ed25519.Sign(priv, signingInput), nil
	default:
		return nil, fmt.Errorf("unsupported jwt algorithm %q", alg)
	}
}

func verifyBytes(alg string, signingInput, sig []byte, secret string) error {
	switch alg {
	case "HS256", "HS384", "HS512":
		expected, err := signBytes(alg, signingInput, secret)
		if err != nil {
			return err
		}
		if !hmac.Equal(sig, expected) {
			return errors.New("invalid jwt signature")
		}
		return nil
	case "RS256", "RS384", "RS512":
		cryptoHash, newHash, err := hashForAlg(alg)
		if err != nil {
			return err
		}
		pub, err := parseRSAPublicKey(secret)
		if err != nil {
			return err
		}
		h := newHash()
		h.Write(signingInput)
		if err := rsa.VerifyPKCS1v15(pub, cryptoHash, h.Sum(nil), sig); err != nil {
			return errors.New("invalid jwt signature")
		}
		return nil
	case "ES256", "ES384", "ES512":
		_, newHash, err := hashForAlg(alg)
		if err != nil {
			return err
		}
		pub, err := parseECDSAPublicKey(secret)
		if err != nil {
			return err
		}
		keyBytes := (pub.Curve.Params().BitSize + 7) / 8
		if len(sig) == 2*keyBytes {
			r := new(big.Int).SetBytes(sig[:keyBytes])
			s := new(big.Int).SetBytes(sig[keyBytes:])
			h := newHash()
			h.Write(signingInput)
			if !ecdsa.Verify(pub, h.Sum(nil), r, s) {
				return errors.New("invalid jwt signature")
			}
			return nil
		}
		// Fallback for ASN.1 DER encoded ECDSA signatures
		var esig struct{ R, S *big.Int }
		if _, err := asn1.Unmarshal(sig, &esig); err != nil {
			return errors.New("invalid jwt signature")
		}
		h := newHash()
		h.Write(signingInput)
		if !ecdsa.Verify(pub, h.Sum(nil), esig.R, esig.S) {
			return errors.New("invalid jwt signature")
		}
		return nil
	case "EDDSA":
		pub, err := parseEd25519PublicKey(secret)
		if err != nil {
			return err
		}
		if !ed25519.Verify(pub, signingInput, sig) {
			return errors.New("invalid jwt signature")
		}
		return nil
	default:
		return fmt.Errorf("unsupported jwt algorithm %q", alg)
	}
}

func decodePEMBlock(keyStr string) (*pem.Block, error) {
	block, _ := pem.Decode([]byte(keyStr))
	if block == nil {
		return nil, errors.New("failed to decode PEM key")
	}
	return block, nil
}

func parseRSAPrivateKey(keyStr string) (*rsa.PrivateKey, error) {
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RSA private key: %w", err)
	}
	rsaKey, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an RSA private key")
	}
	return rsaKey, nil
}

func parseRSAPublicKey(keyStr string) (*rsa.PublicKey, error) {
	if priv, err := parseRSAPrivateKey(keyStr); err == nil {
		return &priv.PublicKey, nil
	}
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}
	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if rsaPub, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
	}
	return nil, errors.New("failed to parse RSA public key")
}

func parseECDSAPrivateKey(keyStr string) (*ecdsa.PrivateKey, error) {
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	if k, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ECDSA private key: %w", err)
	}
	ecKey, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an ECDSA private key")
	}
	return ecKey, nil
}

func parseECDSAPublicKey(keyStr string) (*ecdsa.PublicKey, error) {
	if priv, err := parseECDSAPrivateKey(keyStr); err == nil {
		return &priv.PublicKey, nil
	}
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if ecPub, ok := pub.(*ecdsa.PublicKey); ok {
			return ecPub, nil
		}
	}
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if ecPub, ok := cert.PublicKey.(*ecdsa.PublicKey); ok {
			return ecPub, nil
		}
	}
	_ = elliptic.P256()
	return nil, errors.New("failed to parse ECDSA public key")
}

func parseEd25519PrivateKey(keyStr string) (ed25519.PrivateKey, error) {
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ed25519 private key: %w", err)
	}
	edKey, ok := k.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("key is not an Ed25519 private key")
	}
	return edKey, nil
}

func parseEd25519PublicKey(keyStr string) (ed25519.PublicKey, error) {
	if priv, err := parseEd25519PrivateKey(keyStr); err == nil {
		return priv.Public().(ed25519.PublicKey), nil
	}
	block, err := decodePEMBlock(keyStr)
	if err != nil {
		return nil, err
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if edPub, ok := pub.(ed25519.PublicKey); ok {
			return edPub, nil
		}
	}
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		if edPub, ok := cert.PublicKey.(ed25519.PublicKey); ok {
			return edPub, nil
		}
	}
	return nil, errors.New("failed to parse Ed25519 public key")
}

func parseLuaJWTOpts(l *lua.LState, idx int) JWTOptions {
	var opts JWTOptions
	if l.GetTop() < idx {
		return opts
	}
	val := l.Get(idx)
	switch v := val.(type) {
	case lua.LString:
		opts.Algorithm = string(v)
	case *lua.LTable:
		m := LTableToMap(v)
		if alg, ok := m["alg"].(string); ok {
			opts.Algorithm = alg
		}
		if expIn, ok := claimFloat(m["expires_in"]); ok {
			opts.ExpiresIn = expIn
		} else if expIn, ok := claimFloat(m["ttl"]); ok {
			opts.ExpiresIn = expIn
		}
		if hdr, ok := m["header"].(map[string]any); ok {
			opts.Header = hdr
		} else if hdr, ok := m["headers"].(map[string]any); ok {
			opts.Header = hdr
		} else {
			reserved := []string{"alg", "expires_in", "ttl", "header", "headers", "ignore_exp", "no_time_validation"}
			extra := map[string]any{}
			for k, val := range m {
				if !slices.Contains(reserved, k) {
					extra[k] = val
				}
			}
			if len(extra) > 0 {
				opts.Header = extra
			}
		}
		if ignoreExp, ok := m["ignore_exp"].(bool); ok && ignoreExp {
			opts.NoTimeValidation = true
		}
		if noTime, ok := m["no_time_validation"].(bool); ok && noTime {
			opts.NoTimeValidation = true
		}
	}
	return opts
}

func luaJWTSign(l *lua.LState) int {
	claimsTbl := l.CheckTable(1)
	secret := l.OptString(2, "")
	opts := parseLuaJWTOpts(l, 3)

	claims := LTableToMap(claimsTbl)
	token, err := SignJWT(claims, secret, opts)
	if err != nil {
		l.RaiseError("jwt.sign: %v", err)
		return 0
	}
	l.Push(lua.LString(token))
	return 1
}

func luaJWTDecode(l *lua.LState) int {
	token := l.CheckString(1)
	claims, header, sig, err := DecodeJWT(token)
	if err != nil {
		l.Push(lua.LNil)
		l.Push(lua.LNil)
		l.Push(lua.LString(err.Error()))
		return 3
	}
	l.Push(MapToLTable(l, claims))
	l.Push(MapToLTable(l, header))
	l.Push(lua.LString(sig))
	return 3
}

func luaJWTVerify(l *lua.LState) int {
	token := l.CheckString(1)
	secret := l.CheckString(2)
	opts := parseLuaJWTOpts(l, 3)

	claims, header, err := VerifyJWT(token, secret, opts)
	if err != nil {
		l.Push(lua.LNil)
		l.Push(lua.LString(err.Error()))
		return 2
	}
	l.Push(MapToLTable(l, claims))
	l.Push(MapToLTable(l, header))
	return 2
}

func luaJWTIsValid(l *lua.LState) int {
	if l.Get(1) == lua.LNil {
		l.Push(lua.LBool(false))
		return 1
	}
	token := l.CheckString(1)
	if l.GetTop() >= 2 && l.Get(2) != lua.LNil {
		secret := l.CheckString(2)
		opts := parseLuaJWTOpts(l, 3)
		_, _, err := VerifyJWT(token, secret, opts)
		l.Push(lua.LBool(err == nil))
		return 1
	}
	l.Push(lua.LBool(IsJWT(token)))
	return 1
}
