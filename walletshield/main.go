package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"sync"
	"time"

	cbor "github.com/fxamacker/cbor/v2"
	"github.com/katzenpost/hpqc/hash"
	kps "github.com/privacy-ethereum/kps/libs/go"

	"github.com/katzenpost/katzenpost/client/config"
	"github.com/katzenpost/katzenpost/client/thin"
	proxycommon "github.com/katzenpost/katzenpost/quic/proxy/common"
)

var (
	timeout          = 120
	ProxyHTTPService = "http_proxy"

	UserForwardPayloadLength = 2000
	thinClientOnly           = true
	contractAddr             = ""
	configPath               = ""
)

type Server struct {
	log        *log.Logger
	thin       *thin.ThinClient
	configPath string
	logLevel   string
	mu         sync.Mutex
}

func (s *Server) logInfof(format string, args ...interface{})  { s.log.Printf("INFO: "+format, args...) }
func (s *Server) logWarnf(format string, args ...interface{})  { s.log.Printf("WARN: "+format, args...) }
func (s *Server) logErrorf(format string, args ...interface{}) { s.log.Printf("ERROR: "+format, args...) }
func (s *Server) logDebugf(format string, args ...interface{}) { s.log.Printf("DEBUG: "+format, args...) }
func (s *Server) logFatalf(format string, args ...interface{}) { s.log.Fatalf("FATAL: "+format, args...) }

func (s *Server) reconnect() *thin.ThinClient {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfgThin, err := thin.LoadFile(s.configPath)
	if err != nil {
		s.logErrorf("Failed to load config for reconnect: %s", err)
		return s.thin
	}

	logging := &config.Logging{
		Disable: false,
		Level:   s.logLevel,
	}
	client := thin.NewThinClient(cfgThin, logging)
	err = client.Dial()
	if err != nil {
		s.logErrorf("Failed to reconnect: %s", err)
		return s.thin
	}
	s.thin.Close()
	s.thin = client
	s.logInfof("Reconnected to client daemon")
	return client
}

func (s *Server) getThin() *thin.ThinClient {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.thin
}

func main() {
	var logLevel string
	var listenAddr string
	var kpsListenAddr string
	var delayStart int
	var testProbe bool
	var testProbeCount int
	var testProbeResponseDelay int
	var testProbeSendDelay int

	flag.StringVar(&configPath, "config", "", "file path of the thin client configuration TOML file")
	flag.IntVar(&delayStart, "delay_start", 0, "max random seconds to delay start")
	flag.StringVar(&logLevel, "log_level", "DEBUG", "logging level")
	flag.StringVar(&listenAddr, "listen", "", "local socket to listen HTTP on")
	flag.StringVar(&kpsListenAddr, "kps_listen", "", "KPS listen address (e.g. 0.0.0.0:9201)")
	flag.BoolVar(&thinClientOnly, "thin", true, "use thin client mode")
	flag.BoolVar(&testProbe, "probe", false, "send test probes instead of handling requests")
	flag.IntVar(&testProbeCount, "probe_count", 1, "number of test probes to send")
	flag.IntVar(&testProbeResponseDelay, "probe_response_delay", 0, "test probe response delay")
	flag.IntVar(&testProbeSendDelay, "probe_send_delay", 10, "test probe delay between probes")
	flag.IntVar(&timeout, "timeout", timeout, "seconds to wait for a request")
	flag.StringVar(&contractAddr, "contract", "", "specifier contract address for /boot endpoint")
	flag.Parse()

	if listenAddr == "" && !testProbe {
		panic("listen flag must be set")
	}
	if configPath == "" {
		panic("config flag must be set")
	}

	level := logLevel
	mylog := log.New(os.Stdout, "walletshield: ", log.LstdFlags|log.Lshortfile)

	if delayStart > 0 {
		d := rand.Intn(delayStart)
		mylog.Printf("INFO: Delaying start for %d seconds...", d)
		time.Sleep(time.Duration(d) * time.Second)
	}

	cfgThin, err := thin.LoadFile(configPath)
	if err != nil {
		panic(fmt.Errorf("failed to load thin client config: %s", err))
	}

	logging := &config.Logging{
		Disable: false,
		Level:   level,
	}

	thinClient := thin.NewThinClient(cfgThin, logging)
	err = thinClient.Dial()
	if err != nil {
		panic(err)
	}

	server := &Server{
		log:        mylog,
		thin:       thinClient,
		configPath: configPath,
		logLevel:   level,
	}

	go func() {
		eventSink := thinClient.EventSink()
		defer thinClient.StopEventSink(eventSink)
		everConnected := false
		for event := range eventSink {
			switch v := event.(type) {
			case *thin.ConnectionStatusEvent:
				if v.IsConnected {
					everConnected = true
				} else if everConnected {
					mylog.Printf("WARN: Connection lost, attempting reconnect...")
					server.reconnect()
					everConnected = false
				}
			}
		}
		mylog.Printf("WARN: Event sink closed, connection to daemon may be lost")
	}()

	// Start KPS listener alongside HTTP
	if kpsListenAddr != "" {
		go startKPSListener(server, kpsListenAddr, listenAddr)
	}

	if testProbe {
		server.SendTestProbes(testProbeSendDelay, testProbeCount, testProbeResponseDelay)
	} else {
		http.HandleFunc("/", server.Handler)
		http.ListenAndServe(listenAddr, nil)
	}
}

func (s *Server) Handler(w http.ResponseWriter, req *http.Request) {
	s.logInfof("Received HTTP request for %s", req.URL)

	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	if req.URL.Scheme == "" {
		req.URL.Scheme = "https"
	}

	serialized, err := httputil.DumpRequest(req, true)
	if err != nil {
		s.logErrorf("httputil.DumpRequest failed: %s", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	s.logDebugf("RAW HTTP REQUEST:\n%s", string(serialized))

	thin := s.getThin()
	rawReply, err := sendRequest(thin, serialized)
	if err != nil {
		s.logWarnf("Thin client error, reconnecting: %s", err)
		thin = s.reconnect()
		rawReply, err = sendRequest(thin, serialized)
	}
	if err != nil {
		s.logErrorf("Failed to send message: %s", err)
		if strings.Contains(err.Error(), "exceeds maximum") {
			http.Error(w, "custom 500", http.StatusInternalServerError)
		} else {
			http.Error(w, "custom 404", http.StatusNotFound)
		}
		return
	}

	// Decode CBOR response from the server
	proxyResponse := &proxycommon.Response{}
	_, err = cbor.UnmarshalFirst(rawReply, proxyResponse)
	if err != nil {
		s.logErrorf("Failed to unmarshal CBOR response: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	responseReader := bufio.NewReader(bytes.NewReader(proxyResponse.Payload))
	resp, err := http.ReadResponse(responseReader, req)
	if err != nil {
		s.logErrorf("Failed to parse HTTP response: %s", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	s.logInfof("Response: %d %s", resp.StatusCode, resp.Status)

	// Copy response headers
	for k, v := range resp.Header {
		for _, hv := range v {
			w.Header().Add(k, hv)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func sendRequest(thin *thin.ThinClient, httpRequestBytes []byte) ([]byte, error) {
	if len(httpRequestBytes) > UserForwardPayloadLength {
		return nil, fmt.Errorf("payload size %d exceeds maximum %d bytes", len(httpRequestBytes), UserForwardPayloadLength)
	}

	doc := thin.PKIDocument()
	if doc == nil {
		return nil, fmt.Errorf("PKI document is not available")
	}
	log.Printf("PKI doc epoch=%d, num service nodes=%d\n", doc.Epoch, len(doc.ServiceNodes))

	target, err := thin.GetService(ProxyHTTPService)
	if err != nil {
		return nil, fmt.Errorf("GetService(%s) failed: %w", ProxyHTTPService, err)
	}
	nodeId := hash.Sum256(target.MixDescriptor.IdentityKey)
	log.Printf("GetService(%s) ok: endpoint=%s, node=%x\n", ProxyHTTPService, target.RecipientQueueID, nodeId[:8])

	timeoutCtx, cancel := context.WithTimeout(context.TODO(), time.Duration(timeout)*time.Second)
	defer cancel()
	return thin.BlockingSendMessage(timeoutCtx, httpRequestBytes, &nodeId, target.RecipientQueueID)
}

func startKPSListener(s *Server, addr string, httpAddr string) {
	ln, err := kps.Listen(context.Background(), addr, kps.Options{
		KeyFile: "kps.key",
	})
	if err != nil {
		s.logFatalf("KPS listen failed: %s", err)
	}

	kpsAddr := ln.Address("")
	s.logInfof("KPS address: %s", kpsAddr)

	// Register /boot endpoint on HTTP server
	http.HandleFunc("/boot", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"fetchInit": map[string]interface{}{
				"address": contractAddr,
				"preExisting": map[string]interface{}{
					"resolve": fmt.Sprintf("http://%s/get-worker", httpAddr),
				},
			},
			"kpsAddr": kpsAddr,
		})
	})

	// Register /get-worker endpoint
	http.HandleFunc("/get-worker", func(w http.ResponseWriter, r *http.Request) {
		worker, err := os.ReadFile("worker.js")
		if err != nil {
			http.Error(w, "worker bundle not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(worker)))
		w.Write(worker)
	})

	s.logInfof("KPS listener started on %s", addr)

	for {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			s.logErrorf("KPS accept: %s", err)
			continue
		}
		go handleKPSConn(s, conn)
	}
}

func handleKPSConn(s *Server, conn kps.Conn) {
	defer conn.Close()
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		go func() {
			defer stream.Close()
			body, _ := io.ReadAll(stream)

			reply, err := sendRequest(s.getThin(), body)
			if err != nil {
				s.logErrorf("KPS sendRequest: %s", err)
				return
			}

			stream.Write(reply)
			stream.CloseWrite()
		}()
	}
}

func (s *Server) SendTestProbes(testProbeSendDelay int, testProbeCount int, testProbeResponseDelay int) {
	url := fmt.Sprintf("http://nowhere/_/probe/%d", testProbeResponseDelay)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		s.logErrorf("http.NewRequest failed: %s", err)
		return
	}
	buf := new(bytes.Buffer)
	req.Write(buf)
	httpRequestBytes := buf.Bytes()

	var packetsTransmitted, packetsReceived int
	var rttMin, rttMax, rttTotal float64
	rttMin = math.MaxFloat64

	for {
		packetsTransmitted++
		t := time.Now()

		_, err = sendRequest(s.getThin(), httpRequestBytes)
		elapsed := time.Since(t).Seconds()
		if err != nil {
			s.logErrorf("Probe failed after %.2fs: %s", elapsed, err)
		} else {
			packetsReceived++
			rttTotal += elapsed
			if elapsed < rttMin {
				rttMin = elapsed
			}
			if elapsed > rttMax {
				rttMax = elapsed
			}
		}

		packetLoss := float64(packetsTransmitted-packetsReceived) / float64(packetsTransmitted) * 100
		rttAvg := rttTotal / float64(packetsReceived)
		if packetsReceived == 0 {
			rttMin = math.NaN()
		}
		s.logInfof("Probe packet transmitted/received/loss = %d/%d/%.1f%% | rtt min/avg/max = %.2f/%.2f/%.2f s",
			packetsTransmitted, packetsReceived, packetLoss, rttMin, rttAvg, rttMax)

		if testProbeCount != 0 && packetsTransmitted >= testProbeCount {
			os.Exit(0)
		}

		time.Sleep(time.Duration(testProbeSendDelay) * time.Second)
	}
}
