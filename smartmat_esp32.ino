#include <WiFi.h>
#include <DNSServer.h>
#include <WebServer.h>

const char* ap_ssid      = "smartMat32";
const char* ap_password  = "3c627a9a6f22087cebadbab3b832645f61a1eb40";
const IPAddress ap_ip(192, 168, 66, 1);
const IPAddress ap_gw(192, 168, 66, 1);
const IPAddress ap_mask(255, 255, 255, 0);

const char* sta_ssid     = "YOUR_WIFI_SSID";
const char* sta_password = "YOUR_WIFI_PASSWORD";

DNSServer dns;

WebServer serverDevice(80);    // for Smartmat device
WebServer serverMetrics(9100); // for Prometheus

struct Measurement {
  float weight;
  float battery;
  int remaining;
  bool valid;
} lastMeas = {0, 0, 0, false};

String jsonStatus() {
  String j = "{";
  j += "\"station_ip\":\"" + WiFi.localIP().toString() + "\",";
  j += "\"weight\":" + String(lastMeas.weight) + ",";
  j += "\"battery\":" + String(lastMeas.battery) + ",";
  j += "\"remaining\":" + String(lastMeas.remaining);
  j += "}";
  return j;
}

String promMetrics() {
  String body = "# HELP smartmat_weight_grams Current weight in grams\n";
  body += "# TYPE smartmat_weight_grams gauge\n";
  body += "smartmat_weight_grams " + String(lastMeas.weight) + "\n";
  body += "# HELP smartmat_battery_level Battery level (0.0-1.0)\n";
  body += "# TYPE smartmat_battery_level gauge\n";
  body += "smartmat_battery_level " + String(lastMeas.battery) + "\n";
  body += "# HELP smartmat_remaining_percent Remaining stock percent\n";
  body += "# TYPE smartmat_remaining_percent gauge\n";
  body += "smartmat_remaining_percent " + String(lastMeas.remaining) + "\n";
  return body;
}

void parseAndStore(String raw) {
  int wIdx = raw.indexOf("\"w\":\"");
  if (wIdx >= 0) {
    wIdx += 4;
    int end = raw.indexOf("\"", wIdx + 2);
    lastMeas.weight = raw.substring(wIdx + 1, end).toFloat();
  }
  int bIdx = raw.indexOf("\"b\":\"");
  if (bIdx >= 0) {
    bIdx += 4;
    int end = raw.indexOf("\"", bIdx + 2);
    lastMeas.battery = raw.substring(bIdx + 1, end).toFloat();
  }
  int pIdx = raw.indexOf("\"p\":\"");
  if (pIdx >= 0) {
    pIdx += 4;
    int end = raw.indexOf("\"", pIdx + 2);
    lastMeas.remaining = raw.substring(pIdx + 1, end).toInt();
  }
  lastMeas.valid = true;
}

String utcTimestamp() {
  time_t now = time(nullptr);
  struct tm timeinfo;
  gmtime_r(&now, &timeinfo);
  char buf[30];
  strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", &timeinfo);
  return String(buf);
}

// ---- Smartmat device endpoints (port 80) ----

void handleDeviceVersion2I() {
  serverDevice.send(200, "application/json; charset=utf-8", "");
}

void handleDeviceVersion2S() {
  String body = "{\"i\":300,\"c\":\"http://measure.lite.smartmat.io/v1/device/version2\",\"mr\":0,\"mrd\":\"\",\"fr\":0,\"frd\":\"\",\"o\":0,\"md\":0}";
  serverDevice.send(200, "application/json; charset=utf-8", body);
}

void handleDeviceVersion2SD() {
  String body = "{\"d\":\"";
  body += utcTimestamp();
  body += "\",\"tz\":\"UTC\"}";
  serverDevice.send(200, "application/json; charset=utf-8", body);
}

void handleDeviceVersion2M() {
  if (serverDevice.hasArg("plain")) {
    String raw = serverDevice.arg("plain");
    Serial.print("Received: ");
    Serial.println(raw);
    parseAndStore(raw);
  }
  String body = "{\"m\":\"OK\",\"d\":\"";
  body += utcTimestamp();
  body += "\",\"tz\":\"UTC\"}";
  serverDevice.send(200, "application/json; charset=utf-8", body);
}

// ---- Prometheus / management endpoints (port 9100) ----

void handleMetricsRoot() {
  String html = "<html><body>";
  html += "<h1>Smartmat Exporter</h1>";
  html += "<a href=\"/metrics\">/metrics (Prometheus)</a><br>";
  html += "<a href=\"/status\">/status</a>";
  html += "</body></html>";
  serverMetrics.send(200, "text/html", html);
}

void handleMetrics() {
  serverMetrics.send(200, "text/plain; charset=utf-8", promMetrics());
}

void handleStatus() {
  serverMetrics.send(200, "application/json", jsonStatus());
}

void setup() {
  Serial.begin(115200);
  delay(1000);
  Serial.println("\nStarting Smartmat Prometheus Exporter on ESP32...");

  WiFi.mode(WIFI_AP_STA);

  WiFi.softAPConfig(ap_ip, ap_gw, ap_mask);
  WiFi.softAP(ap_ssid, ap_password);
  Serial.print("SoftAP IP: ");
  Serial.println(WiFi.softAPIP());

  WiFi.begin(sta_ssid, sta_password);
  Serial.print("Connecting to WiFi");
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  Serial.println();
  Serial.print("Station IP: ");
  Serial.println(WiFi.localIP());

  dns.start(53, "*", ap_ip);

  // Device-facing server (port 80)
  serverDevice.on("/v1/device/version2/i",  HTTP_POST, handleDeviceVersion2I);
  serverDevice.on("/v1/device/version2/s",  HTTP_POST, handleDeviceVersion2S);
  serverDevice.on("/v1/device/version2/sd", HTTP_GET,  handleDeviceVersion2SD);
  serverDevice.on("/v1/device/version2/m",  HTTP_POST, handleDeviceVersion2M);
  serverDevice.begin();

  // Metrics-facing server (port 9100)
  serverMetrics.on("/",          handleMetricsRoot);
  serverMetrics.on("/metrics",   handleMetrics);
  serverMetrics.on("/status",    handleStatus);
  serverMetrics.begin();

  Serial.println("Device server:  http://" + WiFi.softAPIP().toString() + ":80");
  Serial.println("Metrics server: http://" + WiFi.localIP().toString() + ":9100/metrics");
}

void loop() {
  dns.processNextRequest();
  serverDevice.handleClient();
  serverMetrics.handleClient();
}
