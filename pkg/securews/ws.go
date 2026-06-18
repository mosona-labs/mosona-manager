package secureWS

import (
	"crypto/ecdh"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"

	"github.com/vmihailenco/msgpack/v5"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

type Role int

const (
	RoleHub Role = iota
	RoleAgent
)

type SessionCrypto struct {
	role Role

	aeadTX cipherAead
	aeadRX cipherAead

	txSeq uint64
	rxSeq uint64
}

type cipherAead interface {
	NonceSize() int
	Overhead() int
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

func deriveSessionKeys(
	role Role,
	xPeerPub *ecdh.PublicKey, // Peer Temp X25519 pub
	xLocalPriv *ecdh.PrivateKey, // Local Temp X25519 priv
	hubNonce string,
	agentNonce string,
) (txKey, rxKey []byte, err error) {
	shared, err := xLocalPriv.ECDH(xPeerPub)
	if err != nil {
		return nil, nil, err
	}

	salt := sha256.Sum256([]byte(hubNonce + "|" + agentNonce))
	info := []byte("mosona-secure-ws-v1")

	okm := make([]byte, 64)
	r := hkdf.New(sha256.New, shared, salt[:], info)
	if _, err := io.ReadFull(r, okm); err != nil {
		return nil, nil, err
	}

	hubToAgent := okm[:32]
	agentToHub := okm[32:64]

	if role == RoleHub {
		return hubToAgent, agentToHub, nil
	}
	return agentToHub, hubToAgent, nil
}

func NewSessionCrypto(
	role Role,
	xPeerPub *ecdh.PublicKey,
	xLocalPriv *ecdh.PrivateKey,
	hubNonce, agentNonce string,
) (*SessionCrypto, error) {
	txKey, rxKey, err := deriveSessionKeys(role, xPeerPub, xLocalPriv, hubNonce, agentNonce)
	if err != nil {
		return nil, err
	}

	aeadTX, err := chacha20poly1305.New(txKey)
	if err != nil {
		return nil, err
	}
	aeadRX, err := chacha20poly1305.New(rxKey)
	if err != nil {
		return nil, err
	}

	return &SessionCrypto{
		role:   role,
		aeadTX: aeadTX,
		aeadRX: aeadRX,
		txSeq:  0,
		rxSeq:  0,
	}, nil
}

func makeNonce(role Role, seq uint64) []byte {
	nonce := make([]byte, chacha20poly1305.NonceSize)
	var tag byte
	if role == RoleHub {
		tag = 0xA1
	} else {
		tag = 0xB2
	}
	nonce[0] = tag
	// nonce[1..3] = 0
	binary.BigEndian.PutUint64(nonce[4:], seq)
	return nonce
}

func (sc *SessionCrypto) Encrypt(plain []byte) ([]byte, error) {
	seq := sc.txSeq
	sc.txSeq++

	nonce := makeNonce(sc.role, seq)

	aad := make([]byte, 8)
	binary.BigEndian.PutUint64(aad, seq)

	ct := sc.aeadTX.Seal(nil, nonce, plain, aad)
	data, err := msgpack.Marshal(SecureFrame{Seq: seq, Ciphertext: ct})
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (sc *SessionCrypto) Decrypt(bf []byte) ([]byte, error) {
	var f SecureFrame
	if err := msgpack.Unmarshal(bf, &f); err != nil {
		return nil, err
	}

	if f.Seq != sc.rxSeq {
		return nil, errors.New("bad seq (replay or out-of-order)")
	}
	sc.rxSeq++

	nonce := makeNonce(oppositeRole(sc.role), f.Seq)

	aad := make([]byte, 8)
	binary.BigEndian.PutUint64(aad, f.Seq)

	pt, err := sc.aeadRX.Open(nil, nonce, f.Ciphertext, aad)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

func oppositeRole(r Role) Role {
	if r == RoleHub {
		return RoleAgent
	}
	return RoleHub
}
