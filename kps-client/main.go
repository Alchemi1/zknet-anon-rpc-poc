package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	kps "github.com/privacy-ethereum/kps/libs/go"
)

func main() {
	bootURL := flag.String("boot", "http://127.0.0.1:9200", "walletshield boot URL")
	rpcMethod := flag.String("method", "eth_blockNumber", "JSON-RPC method")
	rpcHost := flag.String("rpc-host", "ethereum-sepolia.publicnode.com", "RPC Host header for upstream routing")
	timeoutSec := flag.Int("timeout", 120, "timeout in seconds")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeoutSec)*time.Second)
	defer cancel()

	bootResp, err := http.Get(*bootURL + "/boot")
	if err != nil {
		fmt.Fprintf(os.Stderr, "GET %s/boot: %s\n", *bootURL, err)
		os.Exit(1)
	}
	defer bootResp.Body.Close()

	var boot struct {
		KPSAddr string `json:"kpsAddr"`
	}
	if err := json.NewDecoder(bootResp.Body).Decode(&boot); err != nil {
		fmt.Fprintf(os.Stderr, "decode boot response: %s\n", err)
		os.Exit(1)
	}
	if boot.KPSAddr == "" {
		fmt.Fprintf(os.Stderr, "boot response missing kpsAddr\n")
		os.Exit(1)
	}
	fmt.Printf("KPS address: %s\n", boot.KPSAddr)
	fmt.Printf("Connecting...\n")

	conn, err := kps.Dial(ctx, boot.KPSAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "kps.Dial: %s\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	fmt.Printf("Connected (remote: %s)\n", conn.RemoteAddr())

	stream, err := conn.OpenStream(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenStream: %s\n", err)
		os.Exit(1)
	}
	defer stream.Close()

	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  *rpcMethod,
		"params":  []any{},
		"id":      1,
	}
	bodyBytes, _ := json.Marshal(body)

	var reqBuf bytes.Buffer
	reqBuf.WriteString("POST / HTTP/1.1\r\n")
	reqBuf.WriteString(fmt.Sprintf("Host: %s\r\n", *rpcHost))
	reqBuf.WriteString("Content-Type: application/json\r\n")
	reqBuf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(bodyBytes)))
	reqBuf.WriteString("\r\n")
	reqBuf.Write(bodyBytes)

	fmt.Printf("Sending request (path=/, host=%s, method=%s)...\n", *rpcHost, *rpcMethod)

	if _, err := stream.Write(reqBuf.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "stream.Write: %s\n", err)
		os.Exit(1)
	}
	stream.CloseWrite()

	respBytes, err := io.ReadAll(stream)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stream.Read: %s\n", err)
		os.Exit(1)
	}

	var proxyResp struct {
		Payload []byte `cbor:"Payload"`
	}
	if _, err := cbor.UnmarshalFirst(respBytes, &proxyResp); err != nil {
		fmt.Fprintf(os.Stderr, "cbor unmarshal: %s\n", err)
		fmt.Printf("Raw response (%d bytes):\n%s\n", len(respBytes), string(respBytes))
		os.Exit(1)
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(proxyResp.Payload)), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse HTTP response: %s\n", err)
		fmt.Printf("Raw Payload (%d bytes):\n%s\n", len(proxyResp.Payload), string(proxyResp.Payload))
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("HTTP %d %s\n", resp.StatusCode, resp.Status)

	var rpcResp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		fmt.Printf("Response body: %s\n", string(respBody))
	} else if rpcResp.Error != nil {
		fmt.Printf("RPC error: [%d] %s\n", rpcResp.Error.Code, rpcResp.Error.Message)
	} else {
		fmt.Printf("Result: %s\n", string(rpcResp.Result))
	}
}
