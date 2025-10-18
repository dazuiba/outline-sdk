#!/usr/bin/env bash

# <xbar.title>MacOS Proxy Switcher</xbar.title>
# <xbar.version>v0.1</xbar.version>
# <xbar.author>glowinthedark</xbar.author>
# <xbar.author.github>glowinthedark</xbar.author.github>
# <xbar.desc>Set http and socks5 proxy settings on MacOS.</xbar.desc>
# <xbar.image>https://raw.githubusercontent.com/glowinthedark/bitbar-plugins/macos-proxy-switcher/images/mac-proxy-switcher.png.png</xbar.image>
# <xbar.dependencies></xbar.dependencies>
# <xbar.abouturl>https://github.com/glowinthedark/bitbar-plugins/System/macos-proxy-switcher.1m.sh</xbar.abouturl>

# CONFIGURATION
INTERFACE=Wi-Fi

PROXY_HOST=localhost
HTTP_PROXY_PORT=1080
SOCKS_PROXY_PORT=1079

# END CONFIGURATION

if [[ "$1" = "enable_proxy" ]]; then
  networksetup -setsocksfirewallproxy $INTERFACE $PROXY_HOST $SOCKS_PROXY_PORT
  networksetup -setsocksfirewallproxystate $INTERFACE on
  networksetup -setwebproxy $INTERFACE $PROXY_HOST $HTTP_PROXY_PORT
  networksetup -setwebproxystate $INTERFACE on
  networksetup -setsecurewebproxy $INTERFACE $PROXY_HOST $HTTP_PROXY_PORT
  networksetup -setsecurewebproxystate $INTERFACE on
  exit
fi

if [[ "$1" = "disable_proxy" ]]; then
  networksetup -setsocksfirewallproxystate $INTERFACE off
  networksetup -setwebproxystate $INTERFACE off
  networksetup -setsecurewebproxystate $INTERFACE off
  exit
fi


if [[ "$1" = "edit_this_script" ]]; then
  # use default editor for .sh extension
  # open "$0";
  # explicitly use sublimetext3
  open -b com.sublimetext.3 "$0";
  exit
fi

current_socks5_proxy_status=$(networksetup -getsocksfirewallproxy $INTERFACE | awk 'NR=1{print $2; exit}')
current_http_proxy_status=$(networksetup -getwebproxy $INTERFACE | awk 'NR=1{print $2; exit}')

# PROXY STATUS
if [[ $current_socks5_proxy_status == "Yes" ]] || [[ $current_http_proxy_status == "Yes" ]] ; then
  echo '🇬🇧'
  echo '---'
else
  echo "🇪🇸"
  echo '---'
fi

echo '---'

if [[ $current_socks5_proxy_status == "Yes" ]] || [[ $current_http_proxy_status == "Yes" ]]; then
  echo "✅ PROXY is ON! Click to stop http://$PROXY_HOST:$HTTP_PROXY_PORT (socks:$SOCKS_PROXY_PORT) | bash='$0' color=indianred param1=disable_proxy refresh=true terminal=false"
else
  echo "❌ PROXY is OFF! Click to start http://$PROXY_HOST:$HTTP_PROXY_PORT (socks:$SOCKS_PROXY_PORT) | bash='$0' param1=enable_proxy refresh=true terminal=false"
fi

echo '---'
echo "✏️ Edit this file | bash='$0' param1="edit_this_script" terminal=false"

echo '---'
echo "🔃 Refresh... | refresh=true"
