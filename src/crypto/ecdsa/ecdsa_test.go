// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ecdsa

import (
	"bufio"
	"compress/bzip2"
	"crypto"
	"crypto/elliptic"
	"crypto/internal/boring"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
	"io"
	"math/big"
	"os"
	"strings"
	"testing"
)

func testKeyGeneration(t *testing.T, c elliptic.Curve, tag string) {
	priv, err := GenerateKey(c, rand.Reader)
	if err != nil {
		t.Errorf("%s: error: %s", tag, err)
		return
	}
	if !c.IsOnCurve(priv.PublicKey.X, priv.PublicKey.Y) {
		t.Errorf("%s: public key invalid: %s", tag, err)
	}
}

func TestKeyGeneration(t *testing.T) {
	if !boring.Enabled { // P-224 not supported in RHEL OpenSSL.
		testKeyGeneration(t, elliptic.P224(), "p224")
	}
	testKeyGeneration(t, elliptic.P256(), "p256")

	if testing.Short() && !boring.Enabled {
		return
	}
	testKeyGeneration(t, elliptic.P384(), "p384")
	testKeyGeneration(t, elliptic.P521(), "p521")
}

func BenchmarkSignP256(b *testing.B) {
	b.ResetTimer()
	p256 := elliptic.P256()
	hashed := []byte("testing")
	priv, _ := GenerateKey(p256, rand.Reader)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = Sign(rand.Reader, priv, hashed)
		}
	})
}

func BenchmarkSignP384(b *testing.B) {
	b.ResetTimer()
	p384 := elliptic.P384()
	hashed := []byte("testing")
	priv, _ := GenerateKey(p384, rand.Reader)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _, _ = Sign(rand.Reader, priv, hashed)
		}
	})
}

func BenchmarkVerifyP256(b *testing.B) {
	b.ResetTimer()
	p256 := elliptic.P256()
	hashed := []byte("testing")
	priv, _ := GenerateKey(p256, rand.Reader)
	r, s, _ := Sign(rand.Reader, priv, hashed)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			Verify(&priv.PublicKey, hashed, r, s)
		}
	})
}

func BenchmarkKeyGeneration(b *testing.B) {
	b.ResetTimer()
	p256 := elliptic.P256()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			GenerateKey(p256, rand.Reader)
		}
	})
}

func testSignAndVerify(t *testing.T, c elliptic.Curve, tag string) {
	priv, err := GenerateKey(c, rand.Reader)
	if priv == nil {
		t.Fatal(err)
	}

	hashed := []byte("testing")
	r, s, err := Sign(rand.Reader, priv, hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if !Verify(&priv.PublicKey, hashed, r, s) {
		t.Errorf("%s: Verify failed", tag)
	}

	hashed[0] ^= 0xff
	if Verify(&priv.PublicKey, hashed, r, s) {
		t.Errorf("%s: Verify always works!", tag)
	}
}

func testHashSignAndHashVerify(t *testing.T, c elliptic.Curve, tag string) {
	priv, err := GenerateKey(c, rand.Reader)
	if priv == nil {
		t.Fatal(err)
	}

	msg := []byte("testing")
	h := crypto.SHA256
	r, s, err := HashSign(rand.Reader, priv, msg, h)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if !HashVerify(&priv.PublicKey, msg, r, s, h) {
		t.Errorf("%s: Verify failed", tag)
	}

	msg[0] ^= 0xff
	if HashVerify(&priv.PublicKey, msg, r, s, h) {
		t.Errorf("%s: Verify always works!", tag)
	}
}

func TestSignAndVerify(t *testing.T) {
	if boring.Enabled {
		t.Skip("skipping test in boring mode")
	}
	testSignAndVerify(t, elliptic.P224(), "p224")
	testSignAndVerify(t, elliptic.P256(), "p256")

	if testing.Short() && !boring.Enabled {
		return
	}
	testSignAndVerify(t, elliptic.P384(), "p384")
	testSignAndVerify(t, elliptic.P521(), "p521")
}

func TestHashSignAndHashVerify(t *testing.T) {
	testHashSignAndHashVerify(t, elliptic.P256(), "p256")

	if testing.Short() && !boring.Enabled {
		return
	}
	testHashSignAndHashVerify(t, elliptic.P384(), "p384")
	testHashSignAndHashVerify(t, elliptic.P521(), "p521")
}

func testNonceSafety(t *testing.T, c elliptic.Curve, tag string) {
	priv, _ := GenerateKey(c, rand.Reader)

	hashed := []byte("testing")
	r0, s0, err := Sign(zeroReader, priv, hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	hashed = []byte("testing...")
	r1, s1, err := Sign(zeroReader, priv, hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if s0.Cmp(s1) == 0 {
		// This should never happen.
		t.Errorf("%s: the signatures on two different messages were the same", tag)
	}

	if r0.Cmp(r1) == 0 {
		t.Errorf("%s: the nonce used for two different messages was the same", tag)
	}
}

func testNonceSafetyHash(t *testing.T, c elliptic.Curve, tag string) {
	priv, _ := GenerateKey(c, rand.Reader)

	hashed := []byte("testing")
	r0, s0, err := HashSign(zeroReader, priv, hashed, crypto.SHA256)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	hashed = []byte("testing...")
	r1, s1, err := HashSign(zeroReader, priv, hashed, crypto.SHA256)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if s0.Cmp(s1) == 0 {
		// This should never happen.
		t.Errorf("%s: the signatures on two different messages were the same", tag)
	}

	if r0.Cmp(r1) == 0 {
		t.Errorf("%s: the nonce used for two different messages was the same", tag)
	}
}

func TestNonceSafety(t *testing.T) {
	if !boring.Enabled { // P-224 not supported in RHEL OpenSSL.
		testNonceSafety(t, elliptic.P224(), "p224")
	}
	if boring.Enabled {
		testNonceSafetyHash(t, elliptic.P256(), "p256")
	} else {
		testNonceSafety(t, elliptic.P256(), "p256")
	}

	if testing.Short() && !boring.Enabled {
		return
	}
	if boring.Enabled {
		testNonceSafetyHash(t, elliptic.P384(), "p384")
		testNonceSafetyHash(t, elliptic.P521(), "p521")
	} else {
		testNonceSafety(t, elliptic.P384(), "p384")
		testNonceSafety(t, elliptic.P521(), "p521")
	}
}

func testINDCCA(t *testing.T, c elliptic.Curve, tag string) {
	priv, _ := GenerateKey(c, rand.Reader)

	hashed := []byte("testing")
	r0, s0, err := Sign(rand.Reader, priv, hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	r1, s1, err := Sign(rand.Reader, priv, hashed)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if s0.Cmp(s1) == 0 {
		t.Errorf("%s: two signatures of the same message produced the same result", tag)
	}

	if r0.Cmp(r1) == 0 {
		t.Errorf("%s: two signatures of the same message produced the same nonce", tag)
	}
}

func testINDCCAHash(t *testing.T, c elliptic.Curve, tag string) {
	priv, _ := GenerateKey(c, rand.Reader)

	msg := []byte("testing")
	h := crypto.SHA256
	r0, s0, err := HashSign(rand.Reader, priv, msg, h)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	r1, s1, err := HashSign(rand.Reader, priv, msg, h)
	if err != nil {
		t.Errorf("%s: error signing: %s", tag, err)
		return
	}

	if s0.Cmp(s1) == 0 {
		t.Errorf("%s: two signatures of the same message produced the same result", tag)
	}

	if r0.Cmp(r1) == 0 {
		t.Errorf("%s: two signatures of the same message produced the same nonce", tag)
	}
}

func TestINDCCA(t *testing.T) {
	if !boring.Enabled { // P-224 not supported in RHEL OpenSSL.
		testINDCCA(t, elliptic.P224(), "p224")
	}
	if boring.Enabled {
		testINDCCAHash(t, elliptic.P256(), "p256")
	} else {
		testINDCCA(t, elliptic.P256(), "p256")
	}

	if testing.Short() {
		return
	}
	if boring.Enabled {
		testINDCCAHash(t, elliptic.P384(), "p384")
		testINDCCAHash(t, elliptic.P521(), "p521")
	} else {
		testINDCCA(t, elliptic.P384(), "p384")
		testINDCCA(t, elliptic.P521(), "p521")
	}
}

func fromHex(s string) *big.Int {
	r, ok := new(big.Int).SetString(s, 16)
	if !ok {
		panic("bad hex")
	}
	return r
}

func TestVectors(t *testing.T) {
	// This test runs the full set of NIST test vectors from
	// https://csrc.nist.gov/groups/STM/cavp/documents/dss/186-3ecdsatestvectors.zip
	//
	// The SigVer.rsp file has been edited to remove test vectors for
	// unsupported algorithms and has been compressed.

	if testing.Short() {
		return
	}

	f, err := os.Open("testdata/SigVer.rsp.bz2")
	if err != nil {
		t.Fatal(err)
	}

	buf := bufio.NewReader(bzip2.NewReader(f))

	lineNo := 1
	var h hash.Hash
	var ch crypto.Hash
	var msg []byte
	var hashed []byte
	var r, s *big.Int
	pub := new(PublicKey)

	for {
		line, err := buf.ReadString('\n')
		if len(line) == 0 {
			if err == io.EOF {
				break
			}
			t.Fatalf("error reading from input: %s", err)
		}
		lineNo++
		// Need to remove \r\n from the end of the line.
		if !strings.HasSuffix(line, "\r\n") {
			t.Fatalf("bad line ending (expected \\r\\n) on line %d", lineNo)
		}
		line = line[:len(line)-2]

		if len(line) == 0 || line[0] == '#' {
			continue
		}

		if line[0] == '[' {
			line = line[1 : len(line)-1]
			parts := strings.SplitN(line, ",", 2)

			switch parts[0] {
			case "P-224":
				if boring.Enabled { // P-224 not supported in RHEL OpenSSL.
					continue
				}
				pub.Curve = elliptic.P224()
			case "P-256":
				pub.Curve = elliptic.P256()
			case "P-384":
				pub.Curve = elliptic.P384()
			case "P-521":
				pub.Curve = elliptic.P521()
			default:
				pub.Curve = nil
			}

			switch parts[1] {
			case "SHA-1":
				h = sha1.New()
				ch = crypto.SHA1
			case "SHA-224":
				h = sha256.New224()
				ch = crypto.SHA224
			case "SHA-256":
				h = sha256.New()
				ch = crypto.SHA256
			case "SHA-384":
				h = sha512.New384()
				ch = crypto.SHA384
			case "SHA-512":
				h = sha512.New()
				ch = crypto.SHA512
			default:
				h = nil
			}

			continue
		}

		if h == nil || pub.Curve == nil {
			continue
		}

		switch {
		case strings.HasPrefix(line, "Msg = "):
			if msg, err = hex.DecodeString(line[6:]); err != nil {
				t.Fatalf("failed to decode message on line %d: %s", lineNo, err)
			}
		case strings.HasPrefix(line, "Qx = "):
			pub.X = fromHex(line[5:])
		case strings.HasPrefix(line, "Qy = "):
			pub.Y = fromHex(line[5:])
		case strings.HasPrefix(line, "R = "):
			r = fromHex(line[4:])
		case strings.HasPrefix(line, "S = "):
			s = fromHex(line[4:])
		case strings.HasPrefix(line, "Result = "):
			expected := line[9] == 'P'
			h.Reset()
			h.Write(msg)
			hashed := h.Sum(hashed[:0])
			if boring.Enabled {
				if HashVerify(pub, msg, r, s, ch) != expected {
					t.Fatalf("incorrect result on line %d", lineNo)
				}
			} else {
				if Verify(pub, hashed, r, s) != expected {
					t.Fatalf("incorrect result on line %d", lineNo)
				}
			}
		default:
			t.Fatalf("unknown variable on line %d: %s", lineNo, line)
		}
	}
}

func testNegativeInputs(t *testing.T, curve elliptic.Curve, tag string) {
	key, err := GenerateKey(curve, rand.Reader)
	if err != nil {
		t.Errorf("failed to generate key for %q", tag)
	}

	var hash [32]byte
	r := new(big.Int).SetInt64(1)
	r.Lsh(r, 550 /* larger than any supported curve */)
	r.Neg(r)

	if boring.Enabled {
		if HashVerify(&key.PublicKey, hash[:], r, r, crypto.SHA256) {
			t.Errorf("bogus signature accepted for %q", tag)
		}
	} else {
		if Verify(&key.PublicKey, hash[:], r, r) {
			t.Errorf("bogus signature accepted for %q", tag)
		}
	}
}

func TestNegativeInputs(t *testing.T) {
	if !boring.Enabled { // P-224 not supported in RHEL OpenSSL.
		testNegativeInputs(t, elliptic.P224(), "p224")
	}
	testNegativeInputs(t, elliptic.P256(), "p256")
	testNegativeInputs(t, elliptic.P384(), "p384")
	testNegativeInputs(t, elliptic.P521(), "p521")
}

func TestZeroHashSignature(t *testing.T) {
	zeroHash := make([]byte, 64)

	for _, curve := range []elliptic.Curve{elliptic.P224(), elliptic.P256(), elliptic.P384(), elliptic.P521()} {
		if boring.Enabled && curve == elliptic.P224() {
			continue
		}
		privKey, err := GenerateKey(curve, rand.Reader)
		if err != nil {
			panic(err)
		}

		var r, s *big.Int
		// Sign a hash consisting of all zeros.
		if boring.Enabled {
			r, s, err = HashSign(rand.Reader, privKey, zeroHash, crypto.SHA256)
			if err != nil {
				panic(err)
			}
		} else {
			r, s, err = Sign(rand.Reader, privKey, zeroHash)
			if err != nil {
				panic(err)
			}
		}

		// Confirm that it can be verified.
		if boring.Enabled {
			if !HashVerify(&privKey.PublicKey, zeroHash, r, s, crypto.SHA256) {
				t.Errorf("zero hash signature verify failed for %T", curve)
			}
		} else {
			if !Verify(&privKey.PublicKey, zeroHash, r, s) {
				t.Errorf("zero hash signature verify failed for %T", curve)
			}
		}
	}
}
