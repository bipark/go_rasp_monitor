#!/bin/bash

# WiFi/AP 모드 토글 스크립트
# 라즈베리파이 5용

STATE_FILE="/var/lib/wifi_mode_state"
AP_SSID="RaspberryPi_AP"
AP_PASSWORD="raspberry123"
WIFI_SSID="YOUR_SSID"
WIFI_PASSWORD="YOUR_PASSWORD"

# root 권한 확인
if [ "$EUID" -ne 0 ]; then 
    echo "run sudo"
    exit 1
fi

# 현재 상태 확인
get_current_mode() {
    if [ -f "$STATE_FILE" ]; then
        cat "$STATE_FILE"
    else
        echo "wifi"
    fi
}

# AP 모드로 변경
enable_ap_mode() {
    echo "Change to APmode..."
    
    # NetworkManager 사용 (라즈베리파이 OS 최신 버전)
    if command -v nmcli &> /dev/null; then
        # 기존 연결 해제
        nmcli radio wifi on
        nmcli connection down "Hotspot" 2>/dev/null
        
        # AP 생성
        nmcli device wifi hotspot ifname wlan0 ssid "$AP_SSID" password "$AP_PASSWORD"
        
        if [ $? -eq 0 ]; then
            echo "ap" > "$STATE_FILE"
            echo "✓ Complete AP"
            echo "  SSID: $AP_SSID"
            echo "  Password: $AP_PASSWORD"
            echo "  IP: 10.42.0.1"
        else
            echo "✗ APmode Fail"
            return 1
        fi
    else
        # hostapd 사용 (구형 방식)
        systemctl stop wpa_supplicant
        systemctl start hostapd
        systemctl start dnsmasq
        
        echo "ap" > "$STATE_FILE"
        echo "Complete APmode"
    fi
}

# WiFi 클라이언트 모드로 변경
enable_wifi_mode() {
    echo "Change to wifi"
    
    if command -v nmcli &> /dev/null; then
        # AP 해제
        nmcli connection down "Hotspot" 2>/dev/null
        nmcli device disconnect wlan0 2>/dev/null
        
        # WiFi 연결
        sleep 2
        nmcli device wifi connect "$WIFI_SSID" password "$WIFI_PASSWORD"
        
        if [ $? -eq 0 ]; then
            echo "wifi" > "$STATE_FILE"
            echo "Wifi change Complete"
            echo " SSID : $WIFI_SSID"
            IP=$(ip -4 addr show wlan0 | grep -oP '(?<=inet\s)\d+(\.\d+){3}')
            if [ ! -z "$IP" ]; then
                echo "  IP : $IP"
            fi
        else
            echo "WiFi Connect Fail"
            echo "Check SSID/PASS  "
            return 1
        fi
    else
        # wpa_supplicant 사용
        systemctl stop hostapd
        systemctl stop dnsmasq
        systemctl start wpa_supplicant
        
        echo "wifi" > "$STATE_FILE"
        echo "Wifimode Complete"
    fi
}

# 현재 상태 표시
show_status() {
    MODE=$(get_current_mode)
    echo ""
    echo "=========================================="
    echo "mode : $MODE"
    echo "=========================================="
    
    if [ "$MODE" = "ap" ]; then
        echo "Next wifi"
    else
        echo "Nect AP"
    fi
    echo ""
}

# 메인 로직
main() {
    echo ""
    echo "=========================================="
    echo "  Wifi / AP Toggle"
    echo "=========================================="
    echo ""
    
    CURRENT_MODE=$(get_current_mode)
    
    if [ "$CURRENT_MODE" = "wifi" ]; then
        enable_ap_mode
    else
        enable_wifi_mode
    fi
    
    show_status
}

main
