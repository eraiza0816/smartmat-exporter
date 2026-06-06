#!/usr/bin/env bash
set -euo pipefail

SMARTMAT_IP="${SMARTMAT_IP:-$(hostname -I | awk '{print $1}')}"
SMARTMAT_HOST="measure.lite.smartmat.io"
HOSTS_FILE="/etc/hosts"
DNSMASQ_CONF="/etc/dnsmasq.conf"
CREATE_AP_CONF="/etc/create_ap.conf"

backup_file() {
  local f="$1"
  if [ -f "$f" ] && [ ! -f "${f}.smartmat.bak" ]; then
    cp "$f" "${f}.smartmat.bak"
    echo "  backed up $f -> ${f}.smartmat.bak"
  fi
}

restore_file() {
  local f="$1"
  if [ -f "${f}.smartmat.bak" ]; then
    cp "${f}.smartmat.bak" "$f"
    echo "  restored $f from backup"
  fi
}

setup() {
  echo "=== Setting up Smartmat local server (IP: $SMARTMAT_IP) ==="

  # 1. /etc/hosts
  backup_file "$HOSTS_FILE"
  if grep -q "$SMARTMAT_HOST" "$HOSTS_FILE"; then
    echo "  $SMARTMAT_HOST already in /etc/hosts"
  else
    echo "$SMARTMAT_IP	$SMARTMAT_HOST" >> "$HOSTS_FILE"
    echo "  added $SMARTMAT_IP  $SMARTMAT_HOST to /etc/hosts"
  fi

  # 2. dnsmasq local-ttl (device ignores TTL=0)
  backup_file "$DNSMASQ_CONF"
  if grep -q "^local-ttl=" "$DNSMASQ_CONF"; then
    sed -i 's/^local-ttl=.*/local-ttl=60/' "$DNSMASQ_CONF"
    echo "  updated local-ttl=60 in $DNSMASQ_CONF"
  else
    echo "local-ttl=60" >> "$DNSMASQ_CONF"
    echo "  added local-ttl=60 to $DNSMASQ_CONF"
  fi

  # 3. create_ap: ensure ETC_HOSTS=1 so dnsmasq reads /etc/hosts
  if [ -f "$CREATE_AP_CONF" ]; then
    backup_file "$CREATE_AP_CONF"
    if grep -q "^ETC_HOSTS=" "$CREATE_AP_CONF"; then
      sed -i 's/^ETC_HOSTS=.*/ETC_HOSTS=1/' "$CREATE_AP_CONF"
    else
      echo "ETC_HOSTS=1" >> "$CREATE_AP_CONF"
    fi
    echo "  set ETC_HOSTS=1 in $CREATE_AP_CONF"

    # also set ap_max_inactivity for hostapd to prevent early disconnect
    CREATE_AP_BIN="/usr/bin/create_ap"
    if [ -f "$CREATE_AP_BIN" ]; then
      backup_file "$CREATE_AP_BIN"
      if ! grep -q "ap_max_inactivity=60" "$CREATE_AP_BIN"; then
        sed -i '/^create_ap\(\) {/a\  ap_max_inactivity=60' "$CREATE_AP_BIN"
        echo "  added ap_max_inactivity=60 to $CREATE_AP_BIN"
      fi
    fi
  fi

  # 4. restart dnsmasq
  if systemctl is-active --quiet dnsmasq 2>/dev/null; then
    systemctl restart dnsmasq
    echo "  restarted dnsmasq"
  elif systemctl is-active --quiet NetworkManager 2>/dev/null; then
    systemctl restart NetworkManager
    echo "  restarted NetworkManager (dnsmasq runs inside)"
  fi

  echo "=== Setup complete ==="
}

start_go_server() {
  echo "=== Starting Go server on port 80 ==="
  cd "$(dirname "$0")"
  if [ ! -f smartmat_server ]; then
    echo "building smartmat_server..."
    go build -o smartmat_server smartmat_server.go
  fi
  sudo ./smartmat_server
}

teardown() {
  echo "=== Reverting Smartmat local server changes ==="
  restore_file "$HOSTS_FILE"
  restore_file "$DNSMASQ_CONF"
  restore_file "$CREATE_AP_CONF"
  restore_file "/usr/bin/create_ap"

  if systemctl is-active --quiet dnsmasq 2>/dev/null; then
    systemctl restart dnsmasq
    echo "  restarted dnsmasq"
  fi
  echo "=== Teardown complete ==="
}

case "${1:-setup}" in
  setup)
    setup
    ;;
  start)
    start_go_server
    ;;
  teardown|revert)
    teardown
    ;;
  *)
    echo "Usage: $0 {setup|start|teardown}"
    echo "  setup     - configure /etc/hosts and dnsmasq"
    echo "  start     - build & run Go server"
    echo "  teardown  - revert all changes"
    exit 1
    ;;
esac
