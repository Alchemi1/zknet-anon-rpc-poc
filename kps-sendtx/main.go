package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	sececdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/sha3"
)

type rpcClient struct {
	endpoint string
	host     string
}

func (c *rpcClient) call(method string, params []any) (json.RawMessage, error) {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      1,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", c.endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-KPS-Host", c.host)

	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse RPC response: %s", err)
	}
	if len(rpcResp.Error) > 0 && string(rpcResp.Error) != "null" {
		return nil, fmt.Errorf("RPC error: %s", string(rpcResp.Error))
	}
	return rpcResp.Result, nil
}

func parseUint(raw json.RawMessage) (uint64, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return 0, fmt.Errorf("unmarshal hex string: %s", err)
	}
	n, ok := new(big.Int).SetString(s[2:], 16)
	if !ok {
		return 0, fmt.Errorf("bad hex: %s", s)
	}
	return n.Uint64(), nil
}

func privateKeyToAddress(priv *secp256k1.PrivateKey) ([]byte, error) {
	pub := priv.PubKey().SerializeUncompressed()
	if len(pub) != 65 || pub[0] != 0x04 {
		return nil, fmt.Errorf("unexpected pubkey length %d", len(pub))
	}
	h := sha3.NewLegacyKeccak256()
	h.Write(pub[1:])
	hash := h.Sum(nil)
	addr := hash[12:]
	return addr, nil
}

func keccak256(b []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(b)
	return h.Sum(nil)
}

func rlpEncodeBytes(b []byte) []byte {
	if len(b) == 1 && b[0] < 0x80 {
		return b
	}
	return append([]byte{0x80 | byte(len(b))}, b...)
}

func rlpEncodeUint(n uint64) []byte {
	if n == 0 {
		return []byte{0x80}
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte(n & 0xff)}, b...)
		n >>= 8
	}
	return rlpEncodeBytes(b)
}

func rlpEncodeList(items ...[]byte) []byte {
	var body []byte
	for _, it := range items {
		body = append(body, it...)
	}
	if len(body) < 56 {
		return append([]byte{0xc0 | byte(len(body))}, body...)
	}
	lenBytes := big.NewInt(int64(len(body))).Bytes()
	out := []byte{0xf7 + byte(len(lenBytes))}
	out = append(out, lenBytes...)
	return append(out, body...)
}

func main() {
	privHex := flag.String("pk", "", "private key hex (optional; random key used if empty)")
	toAddr := flag.String("to", "0x0000000000000000000000000000000000000000", "recipient address")
	valueWei := flag.Uint64("value", 0, "value in wei")
	gasLimit := flag.Uint64("gas-limit", 21000, "gas limit")
	rpcURL := flag.String("rpc", "http://127.0.0.1:9206/rpc", "kps-monitor /rpc endpoint")
	rpcHost := flag.String("rpc-host", "ethereum-sepolia.publicnode.com", "upstream RPC Host header")
	chainID := flag.Uint64("chain-id", 11155111, "EIP-155 chain id (Sepolia)")
	flag.Parse()

	myLog := log.New(os.Stdout, "kps-sendtx: ", log.LstdFlags)

	var priv *secp256k1.PrivateKey
	if *privHex != "" {
		keyBytes, err := hex.DecodeString(*privHex)
		if err != nil {
			myLog.Fatalf("bad private key hex: %s", err)
		}
		priv = secp256k1.PrivKeyFromBytes(keyBytes)
		myLog.Printf("using provided key")
	} else {
		priv, _ = secp256k1.GeneratePrivateKey()
		myLog.Printf("generated random key (broadcast will need funding)")
	}

	from, err := privateKeyToAddress(priv)
	if err != nil {
		myLog.Fatalf("derive address: %s", err)
	}
	myLog.Printf("sender: 0x%x", from)

	to, err := hex.DecodeString((*toAddr)[2:])
	if err != nil || len(to) != 20 {
		myLog.Fatalf("bad recipient address: %s", *toAddr)
	}
	myLog.Printf("to:     0x%x (value %d wei)", to, *valueWei)

	client := &rpcClient{endpoint: *rpcURL, host: *rpcHost}

	nonceRaw, err := client.call("eth_getTransactionCount", []any{fmt.Sprintf("0x%x", from), "pending"})
	if err != nil {
		myLog.Fatalf("eth_getTransactionCount (via mixnet): %s", err)
	}
	nonce, _ := parseUint(nonceRaw)
	myLog.Printf("nonce:  %d", nonce)

	gasRaw, err := client.call("eth_gasPrice", []any{})
	if err != nil {
		myLog.Fatalf("eth_gasPrice (via mixnet): %s", err)
	}
	gasPrice, _ := parseUint(gasRaw)
	myLog.Printf("gas:    %d wei/gas", gasPrice)

	emptyData := []byte{}
	signed := signLegacyTx(priv, *chainID, nonce, gasPrice, *gasLimit, *valueWei, to, emptyData)
	rawTx := "0x" + hex.EncodeToString(signed)

	myLog.Printf("broadcasting %d-byte signed tx through mixnet...", len(signed))
	start := time.Now()
	txHash, err := client.call("eth_sendRawTransaction", []any{rawTx})
	rtt := time.Since(start)
	if err != nil {
		myLog.Printf("broadcast FAILED (request still reached node via mixnet): %s", err)
		myLog.Printf("fund the sender address on Sepolia then re-run with -pk <key>")
		os.Exit(1)
	}
	var hashStr string
	if err := json.Unmarshal(txHash, &hashStr); err != nil {
		hashStr = string(txHash)
	}
	myLog.Printf("TX BROADCAST OK (round-trip through mixnet: %s)", rtt)
	myLog.Printf("tx hash: %s", hashStr)
	myLog.Printf("view: https://sepolia.etherscan.io/tx/%s", hashStr)
}

func signLegacyTx(priv *secp256k1.PrivateKey, chainID, nonce, gasPrice, gasLimit, value uint64, to, data []byte) []byte {
	signing := rlpEncodeList(
		rlpEncodeUint(nonce),
		rlpEncodeUint(gasPrice),
		rlpEncodeUint(gasLimit),
		rlpEncodeBytes(to),
		rlpEncodeUint(value),
		rlpEncodeBytes(data),
		rlpEncodeUint(chainID),
		rlpEncodeUint(0),
		rlpEncodeUint(0),
	)
	prefixed := append([]byte("\x19Ethereum Signed Message:\n32"), keccak256(signing)...)
	hash := keccak256(prefixed)

	compact := sececdsa.SignCompact(priv, hash, false)
	recID := int(compact[0]) - 27
	v := chainID*2 + 35 + uint64(recID)
	r := compact[1:33]
	s := compact[33:65]

	return rlpEncodeList(
		rlpEncodeUint(nonce),
		rlpEncodeUint(gasPrice),
		rlpEncodeUint(gasLimit),
		rlpEncodeBytes(to),
		rlpEncodeUint(value),
		rlpEncodeBytes(data),
		rlpEncodeUint(v),
		rlpEncodeBytes(r),
		rlpEncodeBytes(s),
	)
}
