package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"sync"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	kps "github.com/privacy-ethereum/kps/libs/go"
)

var (
	connMu      sync.RWMutex
	currentConn kps.Conn
)

type probeResult struct {
	Success bool             `json:"success"`
	RTTMs   int64            `json:"rtt_ms"`
	At      string           `json:"at"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Err     string           `json:"err,omitempty"`
}

type rpcState struct {
	mu sync.RWMutex

	connected    bool
	kpsAddr      string
	startedAt    time.Time
	lastProbeAt  time.Time
	lastRTTMs    int64
	lastResult   json.RawMessage
	minRTTMs     float64
	maxRTTMs     float64
	totalRTTMs   float64
	successCount int
	errorCount   int
	lastErr      string
	history      []probeResult
}

func (s *rpcState) snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	uptime := time.Since(s.startedAt).Seconds()
	avg := float64(0)
	if s.successCount > 0 {
		avg = s.totalRTTMs / float64(s.successCount)
	}
	total := s.successCount + s.errorCount
	successRate := float64(0)
	if total > 0 {
		successRate = float64(s.successCount) / float64(total) * 100
	}
	minRTT := s.minRTTMs
	if math.IsInf(minRTT, 1) {
		minRTT = 0
	}
	return map[string]any{
		"connected":      s.connected,
		"kps_addr":       s.kpsAddr,
		"uptime_seconds": int(uptime),
		"last_probe_at":  s.lastProbeAt.Format(time.RFC3339),
		"last_rtt_ms":    s.lastRTTMs,
		"last_result":    string(s.lastResult),
		"min_rtt_ms":     int(minRTT),
		"max_rtt_ms":     int(s.maxRTTMs),
		"avg_rtt_ms":     int(avg),
		"success_count":  s.successCount,
		"error_count":    s.errorCount,
		"success_rate":   int(successRate),
		"last_error":     s.lastErr,
		"history":        s.history,
	}
}

func (s *rpcState) record(result probeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastProbeAt = time.Now()
	if result.Success {
		rtt := float64(result.RTTMs)
		s.successCount++
		s.lastRTTMs = result.RTTMs
		s.lastResult = result.Result
		s.totalRTTMs += rtt
		if rtt < s.minRTTMs {
			s.minRTTMs = rtt
		}
		if rtt > s.maxRTTMs {
			s.maxRTTMs = rtt
		}
	} else {
		s.errorCount++
		s.lastErr = result.Err
	}
	const maxHistory = 20
	s.history = append(s.history, result)
	if len(s.history) > maxHistory {
		s.history = s.history[len(s.history)-maxHistory:]
	}
}

func fetchKPSAddr(bootURL string) (string, error) {
	resp, err := http.Get(bootURL + "/boot")
	if err != nil {
		return "", fmt.Errorf("get boot: %w", err)
	}
	defer resp.Body.Close()
	var boot struct {
		KPSAddr string `json:"kpsAddr"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&boot); err != nil {
		return "", fmt.Errorf("decode boot: %w", err)
	}
	if boot.KPSAddr == "" {
		return "", errors.New("boot response missing kpsAddr")
	}
	return boot.KPSAddr, nil
}

func buildHTTPRequest(rpcHost string, bodyBytes []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("POST / HTTP/1.1\r\n")
	buf.WriteString(fmt.Sprintf("Host: %s\r\n", rpcHost))
	buf.WriteString("Content-Type: application/json\r\n")
	buf.WriteString(fmt.Sprintf("Content-Length: %d\r\n", len(bodyBytes)))
	buf.WriteString("\r\n")
	buf.Write(bodyBytes)
	return buf.Bytes()
}

func doRPC(conn kps.Conn, rpcHost string, bodyBytes []byte) (result json.RawMessage, rttMs int64, rpcErr string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	start := time.Now()
	stream, err := conn.OpenStream(ctx)
	if err != nil {
		return nil, 0, fmt.Sprintf("OpenStream: %s", err)
	}
	defer stream.Close()
	rawReq := buildHTTPRequest(rpcHost, bodyBytes)
	if _, err := stream.Write(rawReq); err != nil {
		return nil, 0, fmt.Sprintf("Write: %s", err)
	}
	stream.CloseWrite()
	respBytes, err := io.ReadAll(stream)
	rttMs = time.Since(start).Milliseconds()
	if err != nil {
		return nil, rttMs, fmt.Sprintf("Read: %s", err)
	}
	var proxyResp struct {
		Payload []byte `cbor:"Payload"`
	}
	if _, err := cbor.UnmarshalFirst(respBytes, &proxyResp); err != nil {
		return nil, rttMs, fmt.Sprintf("CBOR: %s", err)
	}
	httpResp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(proxyResp.Payload)), nil)
	if err != nil {
		return nil, rttMs, fmt.Sprintf("parse HTTP: %s", err)
	}
	bodyBytes, err = io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
	if err != nil {
		return nil, rttMs, fmt.Sprintf("read body: %s", err)
	}
	var rpcResp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &rpcResp); err != nil {
		return nil, rttMs, fmt.Sprintf("parse RPC: %s", err)
	}
	if rpcResp.Error != nil {
		return nil, rttMs, fmt.Sprintf("RPC error [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, rttMs, ""
}

func main() {
	bootURL := flag.String("boot", "http://127.0.0.1:9200", "walletshield boot URL")
	rpcHost := flag.String("rpc-host", "ethereum-sepolia.publicnode.com", "RPC Host header")
	httpAddr := flag.String("http", ":9206", "monitor HTTP listen address")
	interval := flag.Duration("interval", 30*time.Second, "probe interval")
	flag.Parse()

	myLog := log.New(os.Stdout, "kps-monitor: ", log.LstdFlags)
	st := &rpcState{startedAt: time.Now(), minRTTMs: math.Inf(1)}

	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(st.snapshot())
	})

	http.HandleFunc("/rpc", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method != "POST" {
			http.Error(w, `{"error":"use POST"}`, 405)
			return
		}
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("read body: %s", err)})
			return
		}

		connMu.RLock()
		conn := currentConn
		connMu.RUnlock()
		if conn == nil {
			json.NewEncoder(w).Encode(map[string]any{"error": "no KPS connection"})
			return
		}

		result, rttMs, rpcErr := doRPC(conn, *rpcHost, bodyBytes)
		if rpcErr != "" {
			json.NewEncoder(w).Encode(map[string]any{"error": rpcErr, "rtt_ms": rttMs})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result, "rtt_ms": rttMs})
	})

	go func() {
		myLog.Printf("HTTP on %s/stats and /rpc", *httpAddr)
		if err := http.ListenAndServe(*httpAddr, nil); err != nil {
			myLog.Fatalf("HTTP server: %s", err)
		}
	}()

	for {
		kpsAddr, err := fetchKPSAddr(*bootURL)
		if err != nil {
			myLog.Printf("fetch boot: %s (retry in 5s)", err)
			st.mu.Lock()
			st.connected = false
			st.mu.Unlock()
			time.Sleep(5 * time.Second)
			continue
		}
		myLog.Printf("KPS address: %s", kpsAddr)
		st.mu.Lock()
		st.kpsAddr = kpsAddr
		st.mu.Unlock()

		connMu.Lock()
		if currentConn != nil {
			currentConn.Close()
		}
		connMu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		conn, err := kps.Dial(ctx, kpsAddr)
		cancel()
		if err != nil {
			myLog.Printf("kps.Dial: %s (retry in 5s)", err)
			st.mu.Lock()
			st.connected = false
			st.mu.Unlock()
			time.Sleep(5 * time.Second)
			continue
		}

		connMu.Lock()
		currentConn = conn
		connMu.Unlock()
		st.mu.Lock()
		st.connected = true
		st.mu.Unlock()
		myLog.Printf("Connected")

		for {
			time.Sleep(*interval)
			result, rttMs, rpcErr := doRPC(conn, *rpcHost, buildETHBlockNumberBody())
			if rpcErr != "" {
				myLog.Printf("probe FAIL: %s", rpcErr)
				st.record(probeResult{Success: false, Err: rpcErr})
				myLog.Printf("reconnecting...")
				break
			}
			myLog.Printf("probe OK: %s (%d ms)", string(result), rttMs)
			st.record(probeResult{Success: true, RTTMs: rttMs, Result: result})
		}
	}
}

func buildETHBlockNumberBody() []byte {
	body := map[string]any{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []any{},
		"id":      1,
	}
	b, _ := json.Marshal(body)
	return b
}
