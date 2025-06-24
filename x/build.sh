PATH="$(pwd)/out:$PATH" gomobile bind -ldflags='-s -w' -target=ios,iossimulator,macos -iosversion=13.0 -o "$(pwd)/out/mobileproxy.xcframework" github.com/Jigsaw-Code/outline-sdk/x/mobileproxy
