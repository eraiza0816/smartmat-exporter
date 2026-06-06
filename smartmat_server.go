package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type measurement struct {
	mu        sync.RWMutex
	weight    float64
	battery   float64
	remaining int
	valid     bool
}

var meas = &measurement{}

type settingsResponse struct {
	I   int    `json:"i"`
	C   string `json:"c"`
	Mr  int    `json:"mr"`
	Mrd string `json:"mrd"`
	Fr  int    `json:"fr"`
	Frd string `json:"frd"`
	O   int    `json:"o"`
	Md  int    `json:"md"`
}

type timeResponse struct {
	D  string `json:"d"`
	Tz string `json:"tz"`
}

type measureResponse struct {
	M  string `json:"m"`
	D  string `json:"d"`
	Tz string `json:"tz"`
}

type statusResponse struct {
	StationIP string  `json:"station_ip"`
	Weight    float64 `json:"weight"`
	Battery   float64 `json:"battery"`
	Remaining int     `json:"remaining"`
}

var settings = settingsResponse{
	I: 300, C: "http://measure.lite.smartmat.io/v1/device/version2",
	Mr: 0, Mrd: "", Fr: 0, Frd: "", O: 0, Md: 0,
}

func utcNow() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}

func logBody(r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err == nil && len(body) > 0 {
		log.Printf("POST %s body: %s", r.URL.Path, string(body))
	}
}

func parseAndStore(raw string) {
	meas.mu.Lock()
	defer meas.mu.Unlock()

	type kv struct {
		key, val string
	}
	for _, p := range []kv{{`"w":"`, "weight"}, {`"b":"`, "battery"}, {`"p":"`, "remaining"}} {
		idx := indexOf(raw, p.key)
		if idx < 0 {
			continue
		}
		idx += len(p.key)
		end := indexOf(raw[idx:], `"`)
		if end < 0 {
			continue
		}
		val := raw[idx : idx+end]
		switch p.val {
		case "weight":
			fmt.Sscanf(val, "%f", &meas.weight)
		case "battery":
			fmt.Sscanf(val, "%f", &meas.battery)
		case "remaining":
			fmt.Sscanf(val, "%d", &meas.remaining)
		}
	}
	meas.valid = true
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ---- Device endpoints (port 80) ----

func handleVersion2I(w http.ResponseWriter, r *http.Request) {
	logBody(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte{'"', '"'})
}

func handleVersion2S(w http.ResponseWriter, r *http.Request) {
	logBody(r)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(settings)
}

func handleVersion2SD(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(timeResponse{D: utcNow(), Tz: "UTC"})
}

func handleVersion2M(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err == nil && len(body) > 0 {
		log.Printf("Received: %s", string(body))
		parseAndStore(string(body))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(measureResponse{M: "OK", D: utcNow(), Tz: "UTC"})
}

// ---- Metrics endpoints (port 9100) ----

func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<html><body>`)
	fmt.Fprint(w, `<h1>Smartmat Exporter</h1>`)
	fmt.Fprint(w, `<a href="/metrics">/metrics (Prometheus)</a><br>`)
	fmt.Fprint(w, `<a href="/status">/status</a>`)
	fmt.Fprint(w, `</body></html>`)
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	meas.mu.RLock()
	wgt := meas.weight
	bat := meas.battery
	rem := meas.remaining
	meas.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "# HELP smartmat_weight_grams Current weight in grams\n")
	fmt.Fprintf(w, "# TYPE smartmat_weight_grams gauge\n")
	fmt.Fprintf(w, "smartmat_weight_grams %f\n", wgt)
	fmt.Fprintf(w, "# HELP smartmat_battery_level Battery level (0.0-1.0)\n")
	fmt.Fprintf(w, "# TYPE smartmat_battery_level gauge\n")
	fmt.Fprintf(w, "smartmat_battery_level %f\n", bat)
	fmt.Fprintf(w, "# HELP smartmat_remaining_percent Remaining stock percent\n")
	fmt.Fprintf(w, "# TYPE smartmat_remaining_percent gauge\n")
	fmt.Fprintf(w, "smartmat_remaining_percent %d\n", rem)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	meas.mu.RLock()
	st := statusResponse{
		StationIP: getOutboundIP(),
		Weight:    meas.weight,
		Battery:   meas.battery,
		Remaining: meas.remaining,
	}
	meas.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

func getOutboundIP() string {
	return ""
}

func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next(w, r)
	}
}

func startDeviceServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/device/version2/i", loggingMiddleware(handleVersion2I))
	mux.HandleFunc("/v1/device/version2/s", loggingMiddleware(handleVersion2S))
	mux.HandleFunc("/v1/device/version2/sd", loggingMiddleware(handleVersion2SD))
	mux.HandleFunc("/v1/device/version2/m", loggingMiddleware(handleVersion2M))

	log.Println("Device server starting on :80")
	log.Fatal(http.ListenAndServe(":80", mux))
}

func startMetricsServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRoot)
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/status", handleStatus)

	log.Println("Metrics server starting on :9100")
	log.Fatal(http.ListenAndServe(":9100", mux))
}

func main() {
	go startDeviceServer()
	startMetricsServer()
}
