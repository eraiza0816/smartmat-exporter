# smartmat gateway

スマートマットライト サービス終了に伴うローカルサーバー移植とデータ可視化ツール。

## 構成

```
smartmat_esp32.ino           # ESP32 移植版（AP + Prometheus Exporter）
smartmat_server.go           # Go 版 HTTP サーバー（ラズパイ / Linux 用）
setup_smartmat_server.sh     # Go 版セットアップスクリプト
```

## ESP32 版 (`smartmat_esp32.ino`)

ESP32 一台で SoftAP + DNS 偽装 + 2つのHTTPサーバー（デバイス用・Prometheus用）を同時に動かす。

### アーキテクチャ

```
┌─────────────────┐     SoftAP      ┌──────────────────────────┐
│  Smartmat       │ 192.168.66.x ──→│  ESP32 (SoftAP)          │
│                 │←── DNS偽装───　　│  192.168.66.1:80         │
│                 │  (port 80)      │  serverDevice            │
│                 │                 │                          │
│                 │                 │  ESP32 (Station)         │
│                 │                 │  192.168.10.x:9100       │
│                 │                 │  serverMetrics           │
└─────────────────┘                 └──────────┬───────────────┘
                                               │
                                               │ GET /metrics
                                               ↓
                                      ┌────────────────┐
                                      │  Prometheus     │
                                      │  192.168.10.11  │
                                      └────────────────┘
```

- `WIFI_AP_STA` モードで SoftAP（スマートマット用）と Station（既存WiFi）を同時運用
- `serverDevice` (port 80) — スマートマットからのリクエストを処理
- `serverMetrics` (port 9100) — Prometheus スクレイプ用
- DNS サーバーが `measure.lite.smartmat.io` を SoftAP IP に解決

### エンドポイント

| ポート | パス | 用途 |
|--------|------|------|
| 80 | `/v1/device/version2/i` | デバイス情報 (POST) |
| 80 | `/v1/device/version2/s` | 設定取得 (POST) |
| 80 | `/v1/device/version2/sd` | 時刻取得 (GET) |
| 80 | `/v1/device/version2/m` | 測定データ送信 (POST) |
| 9100 | `/metrics` | Prometheus Exporter |
| 9100 | `/status` | 最新測定値の JSON |
| 9100 | `/` | 簡易トップページ |

### 使い方

1. `sta_ssid` / `sta_password` を環境に合わせて書き換え
2. Arduino IDE / PlatformIO で ESP32 に書き込み
3. スマートマットを SSID `smartMat32` に接続
4. Prometheus の `scrape_configs` に追加:

```yaml
scrape_configs:
  - job_name: 'smartmat'
    static_configs:
      - targets: ['<ESP32のStation IP>:9100']
```

### Prometheus メトリクス

| メトリクス名 | 型 | 説明 |
|-------------|-----|------|
| `smartmat_weight_grams` | gauge | 現在の重量 (g) |
| `smartmat_battery_level` | gauge | バッテリー残量 (0.0–1.0) |
| `smartmat_remaining_percent` | gauge | 在庫残量 (%) |

## Go 版 (`smartmat_server.go`)

ラズパイや Linux マシンで動かす用。DNS 偽装は `/etc/hosts` + dnsmasq に依存。

### 使い方

```bash
# セットアップ（/etc/hosts, dnsmasq 設定）
sudo ./setup_smartmat_server.sh setup

# サーバー起動（ポート 80）
sudo ./setup_smartmat_server.sh start

# 元に戻す
sudo ./setup_smartmat_server.sh teardown
```

環境変数 `SMARTMAT_IP` で IP を指定可能（デフォルト: `hostname -I`）。
