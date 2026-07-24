package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	kps "github.com/privacy-ethereum/kps/libs/go"
)

func main() {
	var kpsListen string
	var upstream string
	var workerFile string
	var httpAddr string
	flag.StringVar(&kpsListen, "kps_listen", "0.0.0.0:9202", "KPS listen address")
	flag.StringVar(&upstream, "upstream", "http://127.0.0.1:64332", "upstream HTTP endpoint")
	flag.StringVar(&workerFile, "worker", "worker.js", "path to worker bundle")
	flag.StringVar(&httpAddr, "http", "127.0.0.1:9203", "HTTP boot endpoint address")
	flag.Parse()

	workerJS, err := os.ReadFile(workerFile)
	if err != nil {
		fmt.Printf("Warning: cannot read worker bundle %s: %s\n", workerFile, err)
	}

	ln, err := kps.Listen(context.Background(), kpsListen, kps.Options{
		KeyFile: "kps.key",
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("KPS relay listening on %s\n", ln.Address(""))
	fmt.Printf("Upstream HTTP: %s\n", upstream)

	go func() {
		http.HandleFunc("/boot", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"fetchInit": map[string]interface{}{
					"address": "0xZKNWalletShield",
					"preExisting": map[string]interface{}{
						"resolve": fmt.Sprintf("http://%s/worker.js", r.Host),
					},
				},
				"kpsAddr": ln.Address(""),
			})
		})
		http.HandleFunc("/worker.js", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/javascript")
			w.Write(workerJS)
		})
		fmt.Printf("HTTP boot endpoint on %s\n", httpAddr)
		http.ListenAndServe(httpAddr, nil)
	}()

	for {
		conn, err := ln.Accept(context.Background())
		if err != nil {
			fmt.Printf("accept error: %s\n", err)
			continue
		}
		go handleConn(conn, upstream)
	}
}

func handleConn(conn kps.Conn, upstream string) {
	defer conn.Close()
	client := &http.Client{Timeout: 120 * time.Second}

	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return
		}
		go func() {
			defer stream.Close()
			body, err := io.ReadAll(stream)
			if err != nil {
				return
			}

			req, err := http.NewRequest("POST", upstream+"/ethereum", bytes.NewReader(body))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				fmt.Printf("upstream error: %s\n", err)
				return
			}
			defer resp.Body.Close()

			respBody, _ := io.ReadAll(resp.Body)
			stream.Write(respBody)
			stream.CloseWrite()
		}()
	}
}
