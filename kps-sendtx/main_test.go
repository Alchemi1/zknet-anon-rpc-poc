package main

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

func TestPrivateKeyToAddressAnvil(t *testing.T) {
	keyBytes, _ := hex.DecodeString("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	priv := secp256k1.PrivKeyFromBytes(keyBytes)
	addr, err := privateKeyToAddress(priv)
	if err != nil {
		t.Fatal(err)
	}
	got := "0x" + hex.EncodeToString(addr)
	want := "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"
	if !strings.EqualFold(got, want) {
		t.Fatalf("address mismatch: got %s want %s", got, want)
	}
}

func TestRLPEncode(t *testing.T) {
	// RLP([0x80, 0x1]) -> 0xc28001
	got := rlpEncodeList([]byte{0x80}, []byte{0x01})
	if hex.EncodeToString(got) != "c28001" {
		t.Fatalf("rlp list mismatch: %x", got)
	}
	// "dog" -> 0x83646f67
	if hex.EncodeToString(rlpEncodeBytes([]byte("dog"))) != "83646f67" {
		t.Fatalf("rlp bytes mismatch")
	}
	// uint 0 -> 0x80, uint 1 -> 0x01
	if hex.EncodeToString(rlpEncodeUint(0)) != "80" {
		t.Fatalf("rlp uint0 mismatch")
	}
	if hex.EncodeToString(rlpEncodeUint(1)) != "01" {
		t.Fatalf("rlp uint1 mismatch")
	}
}
