// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build linux
// +build !android
// +build !no_openssl
// +build !cmd_go_bootstrap
// +build !msan

package boring

// #include "goboringcrypto.h"
import "C"
import (
	"crypto"
	"errors"
	"hash"
	"math/big"
	"runtime"
	"strconv"
	"unsafe"
)

func GenerateKeyRSA(bits int) (N, E, D, P, Q, Dp, Dq, Qinv *big.Int, err error) {
	bad := func(e error) (N, E, D, P, Q, Dp, Dq, Qinv *big.Int, err error) {
		return nil, nil, nil, nil, nil, nil, nil, nil, e
	}

	key := C._goboringcrypto_RSA_new()
	if key == nil {
		return bad(NewOpenSSLError("RSA_new failed"))
	}
	defer C._goboringcrypto_RSA_free(key)

	if C._goboringcrypto_RSA_generate_key_fips(key, C.int(bits), nil) == 0 {
		return bad(NewOpenSSLError("RSA_generate_key_fips failed"))
	}

	var n, e, d, p, q, dp, dq, qinv *C.GO_BIGNUM
	C._goboringcrypto_RSA_get0_key(key, &n, &e, &d)
	C._goboringcrypto_RSA_get0_factors(key, &p, &q)
	C._goboringcrypto_RSA_get0_crt_params(key, &dp, &dq, &qinv)
	return bnToBig(n), bnToBig(e), bnToBig(d), bnToBig(p), bnToBig(q), bnToBig(dp), bnToBig(dq), bnToBig(qinv), nil
}

type PublicKeyRSA struct {
	key *C.GO_RSA
}

func NewPublicKeyRSA(N, E *big.Int) (*PublicKeyRSA, error) {
	key := C._goboringcrypto_RSA_new()
	if key == nil {
		return nil, NewOpenSSLError("RSA_new failed")
	}
	var n, e *C.GO_BIGNUM
	C._goboringcrypto_RSA_get0_key(key, &n, &e, nil)
	if !bigToBn(&n, N) ||
		!bigToBn(&e, E) {
		return nil, NewOpenSSLError("BN_bin2bn failed")
	}
	C._goboringcrypto_RSA_set0_key(key, n, e, nil)
	k := &PublicKeyRSA{key: key}
	// Note: Because of the finalizer, any time k.key is passed to cgo,
	// that call must be followed by a call to runtime.KeepAlive(k),
	// to make sure k is not collected (and finalized) before the cgo
	// call returns.
	runtime.SetFinalizer(k, (*PublicKeyRSA).finalize)
	return k, nil
}

func (k *PublicKeyRSA) finalize() {
	C._goboringcrypto_RSA_free(k.key)
}

type PrivateKeyRSA struct {
	key *C.GO_RSA
}

func NewPrivateKeyRSA(N, E, D, P, Q, Dp, Dq, Qinv *big.Int) (*PrivateKeyRSA, error) {
	key := C._goboringcrypto_RSA_new()
	if key == nil {
		return nil, NewOpenSSLError("RSA_new failed")
	}
	var n, e, d, p, q, dp, dq, qinv *C.GO_BIGNUM
	C._goboringcrypto_RSA_get0_key(key, &n, &e, &d)
	C._goboringcrypto_RSA_get0_factors(key, &p, &q)
	C._goboringcrypto_RSA_get0_crt_params(key, &dp, &dq, &qinv)
	if !bigToBn(&n, N) ||
		!bigToBn(&e, E) ||
		!bigToBn(&d, D) ||
		!bigToBn(&p, P) ||
		!bigToBn(&q, Q) ||
		!bigToBn(&dp, Dp) ||
		!bigToBn(&dq, Dq) ||
		!bigToBn(&qinv, Qinv) {
		return nil, NewOpenSSLError("BN_bin2bn failed")
	}
	C._goboringcrypto_RSA_set0_key(key, n, e, d)
	C._goboringcrypto_RSA_set0_factors(key, p, q)
	C._goboringcrypto_RSA_set0_crt_params(key, dp, dq, qinv)
	k := &PrivateKeyRSA{key: key}
	// Note: Because of the finalizer, any time k.key is passed to cgo,
	// that call must be followed by a call to runtime.KeepAlive(k),
	// to make sure k is not collected (and finalized) before the cgo
	// call returns.
	runtime.SetFinalizer(k, (*PrivateKeyRSA).finalize)
	return k, nil
}

func (k *PrivateKeyRSA) finalize() {
	C._goboringcrypto_RSA_free(k.key)
}

func setupRSA(key *C.GO_RSA,
	padding C.int, h hash.Hash, label []byte, saltLen int, ch crypto.Hash,
	init func(*C.GO_EVP_PKEY_CTX) C.int) (pkey *C.GO_EVP_PKEY, ctx *C.GO_EVP_PKEY_CTX, err error) {
	defer func() {
		if err != nil {
			if pkey != nil {
				C._goboringcrypto_EVP_PKEY_free(pkey)
				pkey = nil
			}
			if ctx != nil {
				C._goboringcrypto_EVP_PKEY_CTX_free(ctx)
				ctx = nil
			}
		}
	}()

	pkey = C._goboringcrypto_EVP_PKEY_new()
	if pkey == nil {
		return nil, nil, NewOpenSSLError("EVP_PKEY_new failed")
	}
	if C._goboringcrypto_EVP_PKEY_set1_RSA(pkey, key) == 0 {
		return nil, nil, NewOpenSSLError("EVP_PKEY_set1_RSA failed")
	}
	ctx = C._goboringcrypto_EVP_PKEY_CTX_new(pkey, nil)
	if ctx == nil {
		return nil, nil, NewOpenSSLError("EVP_PKEY_CTX_new failed")
	}
	if init(ctx) == 0 {
		return nil, nil, NewOpenSSLError("EVP_PKEY_operation_init failed")
	}
	if C._goboringcrypto_EVP_PKEY_CTX_set_rsa_padding(ctx, padding) == 0 {
		return nil, nil, NewOpenSSLError("EVP_PKEY_CTX_set_rsa_padding failed")
	}
	if padding == C.GO_RSA_PKCS1_OAEP_PADDING {
		md := hashToMD(h)
		if md == nil {
			return nil, nil, errors.New("crypto/rsa: unsupported hash function")
		}
		if C._goboringcrypto_EVP_PKEY_CTX_set_rsa_oaep_md(ctx, md) == 0 {
			return nil, nil, NewOpenSSLError("EVP_PKEY_set_rsa_oaep_md failed")
		}
		// ctx takes ownership of label, so malloc a copy for BoringCrypto to free.
		clabel := (*C.uint8_t)(C.malloc(C.size_t(len(label))))
		if clabel == nil {
			return nil, nil, NewOpenSSLError("malloc failed")
		}
		copy((*[1 << 30]byte)(unsafe.Pointer(clabel))[:len(label)], label)
		if C._goboringcrypto_EVP_PKEY_CTX_set0_rsa_oaep_label(ctx, clabel, C.size_t(len(label))) == 0 {
			return nil, nil, NewOpenSSLError("EVP_PKEY_CTX_set0_rsa_oaep_label failed")
		}
	}
	if padding == C.GO_RSA_PKCS1_PSS_PADDING {
		if saltLen != 0 {
			if C._goboringcrypto_EVP_PKEY_CTX_set_rsa_pss_saltlen(ctx, C.int(saltLen)) == 0 {
				return nil, nil, NewOpenSSLError("EVP_PKEY_set_rsa_pss_saltlen failed")
			}
		}
		md := cryptoHashToMD(ch)
		if md == nil {
			return nil, nil, errors.New("crypto/rsa: unsupported hash function")
		}
		if C._goboringcrypto_EVP_PKEY_CTX_set_rsa_mgf1_md(ctx, md) == 0 {
			return nil, nil, NewOpenSSLError("EVP_PKEY_set_rsa_mgf1_md failed")
		}
	}

	return pkey, ctx, nil
}

func cryptRSA(gokey interface{}, key *C.GO_RSA,
	padding C.int, h hash.Hash, label []byte, saltLen int, ch crypto.Hash,
	init func(*C.GO_EVP_PKEY_CTX) C.int,
	crypt func(*C.GO_EVP_PKEY_CTX, *C.uint8_t, *C.uint, *C.uint8_t, C.uint) C.int,
	in []byte) ([]byte, error) {

	pkey, ctx, err := setupRSA(key, padding, h, label, saltLen, ch, init)
	if err != nil {
		return nil, err
	}
	defer C._goboringcrypto_EVP_PKEY_free(pkey)
	defer C._goboringcrypto_EVP_PKEY_CTX_free(ctx)

	var outLen C.uint
	if crypt(ctx, nil, &outLen, base(in), C.uint(len(in))) == 0 {
		return nil, NewOpenSSLError("EVP_PKEY_decrypt/encrypt failed")
	}
	out := make([]byte, outLen)
	if crypt(ctx, base(out), &outLen, base(in), C.uint(len(in))) <= 0 {
		return nil, NewOpenSSLError("EVP_PKEY_decrypt/encrypt failed")
	}
	runtime.KeepAlive(gokey) // keep key from being freed before now
	return out[:outLen], nil
}

func DecryptRSAOAEP(h hash.Hash, priv *PrivateKeyRSA, ciphertext, label []byte) ([]byte, error) {
	return cryptRSA(priv, priv.key, C.GO_RSA_PKCS1_OAEP_PADDING, h, label, 0, 0, decryptInit, decrypt, ciphertext)
}

func EncryptRSAOAEP(h hash.Hash, pub *PublicKeyRSA, msg, label []byte) ([]byte, error) {
	return cryptRSA(pub, pub.key, C.GO_RSA_PKCS1_OAEP_PADDING, h, label, 0, 0, encryptInit, encrypt, msg)
}

func DecryptRSAPKCS1(priv *PrivateKeyRSA, ciphertext []byte) ([]byte, error) {
	return cryptRSA(priv, priv.key, C.GO_RSA_PKCS1_PADDING, nil, nil, 0, 0, decryptInit, decrypt, ciphertext)
}

func EncryptRSAPKCS1(pub *PublicKeyRSA, msg []byte) ([]byte, error) {
	return cryptRSA(pub, pub.key, C.GO_RSA_PKCS1_PADDING, nil, nil, 0, 0, encryptInit, encrypt, msg)
}

func DecryptRSANoPadding(priv *PrivateKeyRSA, ciphertext []byte) ([]byte, error) {
	return cryptRSA(priv, priv.key, C.GO_RSA_NO_PADDING, nil, nil, 0, 0, decryptInit, decrypt, ciphertext)
}

func EncryptRSANoPadding(pub *PublicKeyRSA, msg []byte) ([]byte, error) {
	return cryptRSA(pub, pub.key, C.GO_RSA_NO_PADDING, nil, nil, 0, 0, encryptInit, encrypt, msg)
}

// These dumb wrappers work around the fact that cgo functions cannot be used as values directly.

func decryptInit(ctx *C.GO_EVP_PKEY_CTX) C.int {
	return C._goboringcrypto_EVP_PKEY_decrypt_init(ctx)
}

func decrypt(ctx *C.GO_EVP_PKEY_CTX, out *C.uint8_t, outLen *C.uint, in *C.uint8_t, inLen C.uint) C.int {
	return C._goboringcrypto_EVP_PKEY_decrypt(ctx, out, outLen, in, inLen)
}

func encryptInit(ctx *C.GO_EVP_PKEY_CTX) C.int {
	return C._goboringcrypto_EVP_PKEY_encrypt_init(ctx)
}

func encrypt(ctx *C.GO_EVP_PKEY_CTX, out *C.uint8_t, outLen *C.uint, in *C.uint8_t, inLen C.uint) C.int {
	return C._goboringcrypto_EVP_PKEY_encrypt(ctx, out, outLen, in, inLen)
}

func SignRSAPSS(priv *PrivateKeyRSA, h crypto.Hash, hashed []byte, saltLen int) ([]byte, error) {
	md := cryptoHashToMD(h)
	if md == nil {
		return nil, errors.New("crypto/rsa: unsupported hash function")
	}
	if saltLen == 0 {
		saltLen = -1
	}
	out := make([]byte, C._goboringcrypto_RSA_size(priv.key))
	var outLen C.uint
	if C._goboringcrypto_RSA_sign_pss_mgf1(
		priv.key,
		&outLen, base(out), C.uint(len(out)),
		base(hashed), C.uint(len(hashed)),
		md, nil, C.int(saltLen)) == 0 {
		return nil, NewOpenSSLError("RSA_sign_pss_mgf1 failed")
	}
	runtime.KeepAlive(priv)

	return out[:outLen], nil
}

func VerifyRSAPSS(pub *PublicKeyRSA, h crypto.Hash, hashed, sig []byte, saltLen int) error {
	md := cryptoHashToMD(h)
	if md == nil {
		return errors.New("crypto/rsa: unsupported hash function")
	}
	if saltLen == 0 {
		saltLen = -2 // auto-recover
	}
	if C._goboringcrypto_RSA_verify_pss_mgf1(pub.key,
		base(hashed),
		C.uint(len(hashed)),
		md, nil, C.int(saltLen), base(sig), C.uint(len(sig))) == 0 {
		return NewOpenSSLError("RSA_verify_pss_mgf1 failed")
	}
	runtime.KeepAlive(pub)
	return nil
}

func SignRSAPKCS1v15(priv *PrivateKeyRSA, h crypto.Hash, msg []byte, msgIsHashed bool) ([]byte, error) {
	out := make([]byte, C._goboringcrypto_RSA_size(priv.key))

	md := cryptoHashToMD(h)
	if md == nil {
		return nil, errors.New("crypto/rsa: unsupported hash function: " + strconv.Itoa(int(h)))
	}

	var outLen C.uint

	if msgIsHashed {
		PanicIfStrictFIPS("You must provide a raw unhashed message for PKCS1v15 signing and use HashSignPKCS1v15 instead of SignPKCS1v15")
		nid := C._goboringcrypto_EVP_MD_type(md)
		if C._goboringcrypto_RSA_sign(nid, base(msg), C.uint(len(msg)), base(out), &outLen, priv.key) == 0 {
			return nil, NewOpenSSLError("RSA_sign failed")
		}
		runtime.KeepAlive(priv)
		return out[:outLen], nil
	}

	if C._goboringcrypto_EVP_RSA_sign(md, base(msg), C.uint(len(msg)), base(out), &outLen, priv.key) == 0 {
		return nil, NewOpenSSLError("RSA_sign failed")
	}
	runtime.KeepAlive(priv)
	return out[:outLen], nil
}

func VerifyRSAPKCS1v15(pub *PublicKeyRSA, h crypto.Hash, msg, sig []byte, msgIsHashed bool) error {
	size := int(C._goboringcrypto_RSA_size(pub.key))
	if len(sig) < size {
		// BoringCrypto requires sig to be same size as RSA key, so pad with leading zeros.
		zsig := make([]byte, size)
		copy(zsig[len(zsig)-len(sig):], sig)
		sig = zsig
	}

	md := cryptoHashToMD(h)
	if md == nil {
		return errors.New("crypto/rsa: unsupported hash function")
	}

	if msgIsHashed {
		PanicIfStrictFIPS("You must provide a raw unhashed message for PKCS1v15 verification and use HashVerifyPKCS1v15 instead of VerifyPKCS1v15")
		nid := C._goboringcrypto_EVP_MD_type(md)
		if C._goboringcrypto_RSA_verify(nid, base(msg), C.uint(len(msg)), base(sig), C.uint(len(sig)), pub.key) == 0 {
			return NewOpenSSLError("RSA_verify failed")
		}
		runtime.KeepAlive(pub)
		return nil
	}

	if C._goboringcrypto_EVP_RSA_verify(md, base(msg), C.uint(len(msg)), base(sig), C.uint(len(sig)), pub.key) == 0 {
		return NewOpenSSLError("RSA_verify failed")
	}
	runtime.KeepAlive(pub)
	return nil
}
